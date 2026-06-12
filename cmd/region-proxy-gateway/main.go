package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/admin"
	"github.com/iceqi/region-proxy-gateway/internal/channel"
	"github.com/iceqi/region-proxy-gateway/internal/config"
	"github.com/iceqi/region-proxy-gateway/internal/connection"
	"github.com/iceqi/region-proxy-gateway/internal/deeptest"
	"github.com/iceqi/region-proxy-gateway/internal/ipinfo"
	"github.com/iceqi/region-proxy-gateway/internal/node"
	"github.com/iceqi/region-proxy-gateway/internal/nodecheck"
	"github.com/iceqi/region-proxy-gateway/internal/proxy"
	"github.com/iceqi/region-proxy-gateway/internal/storage"
	"github.com/iceqi/region-proxy-gateway/internal/tunnel"
	"github.com/iceqi/region-proxy-gateway/internal/vpngate"
)

var version = "dev"

type services struct {
	admin      *admin.Server
	nodes      *node.Store
	channels   *channel.Manager
	tracker    *connection.Tracker
	storage    *storage.Store
	proxies    []*proxy.Server
	listeners  []net.Listener
	configPath string
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version)
		return
	}

	cfgPath := filepath.Join("data", "config.json")
	cfg, err := config.LoadOrCreate(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	services, err := buildServices(ctx, cfg, cfgPath)
	if err != nil {
		log.Fatalf("build services: %v", err)
	}
	defer services.channels.Stop(ctx)
	defer services.storage.Close()

	for _, p := range services.proxies {
		listener, err := net.Listen("tcp", p.ListenAddr)
		if err != nil {
			log.Fatalf("proxy listen on %s: %v", p.ListenAddr, err)
		}
		services.listeners = append(services.listeners, listener)
		go func(server *proxy.Server, ln net.Listener) {
			if err := server.Serve(ln); err != nil {
				log.Printf("proxy channel %s stopped: %v", server.ChannelID, err)
			}
		}(p, listener)
		log.Printf("proxy channel %s listening on %s", p.ChannelID, p.ListenAddr)
	}

	adminAddr := fmt.Sprintf("%s:%d", cfg.AdminHost, cfg.AdminPort)
	log.Printf("admin listening on http://%s", adminAddr)
	adminServer := &http.Server{Addr: adminAddr, Handler: services.admin}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = adminServer.Shutdown(shutdownCtx)
		for _, listener := range services.listeners {
			_ = listener.Close()
		}
	}()
	if err := adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func buildServices(ctx context.Context, cfg config.Config, cfgPath string) (services, error) {
	if err := cfg.Validate(); err != nil {
		return services{}, err
	}

	database, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		return services{}, err
	}
	if err := database.MigrateChannels(ctx, cfg.Channels); err != nil {
		_ = database.Close()
		return services{}, err
	}
	if cfgPath != "" && len(cfg.Channels) > 0 {
		cfg.Channels = nil
		if err := config.Save(cfgPath, cfg); err != nil {
			_ = database.Close()
			return services{}, err
		}
	}
	channels, err := database.ListChannels(ctx)
	if err != nil {
		_ = database.Close()
		return services{}, err
	}

	nodes := node.NewStore()
	loadedNodes, err := database.ListNodes(ctx)
	if err != nil {
		_ = database.Close()
		return services{}, err
	}
	if len(loadedNodes) == 0 {
		loadedNodes, err = loadNodes(ctx, cfg)
		if err != nil {
			_ = database.Close()
			return services{}, err
		}
		if err := database.ReplaceNodes(ctx, loadedNodes); err != nil {
			log.Printf("cache nodes failed: %v", err)
		}
	}
	nodes.Replace(loadedNodes)

	tracker := connection.NewTracker()
	checker := nodecheck.Checker{Timeout: 3 * time.Second}
	refreshNodes := func(ctx context.Context) error {
		loaded, err := loadNodes(ctx, cfg)
		if err != nil {
			return err
		}
		nodes.Replace(loaded)
		if err := database.ReplaceNodes(ctx, loaded); err != nil {
			log.Printf("cache refreshed nodes failed: %v", err)
		}
		return nil
	}
	manager := channel.NewManager(channel.Config{
		Channels:      channels,
		Nodes:         nodes,
		TunnelFactory: tunnelFactory(cfg),
		NodeChecker:   checker.Check,
		RefreshNodes:  refreshNodes,
		History:       channelHistoryAdapter{store: database},
		DataDir:       cfg.DataDir,
		OpenVPNCmd:    cfg.OpenVPNCommand,
	})
	if err := manager.Start(ctx); err != nil {
		return services{}, err
	}
	startNodeUpdater(ctx, cfg, nodes, database)
	if err := database.ResetRunningDeepTestJobs(ctx); err != nil {
		log.Printf("reset running deep test jobs failed: %v", err)
	}
	deepWorker := deeptest.NewWorker(deeptest.Config{
		Queue:       database,
		Nodes:       nodes,
		Tester:      deeptest.OpenVPNTester{DataDir: cfg.DataDir, Command: cfg.OpenVPNCommand},
		BatchSize:   1,
		Concurrency: 1,
		Interval:    3 * time.Second,
		Timeout:     25 * time.Second,
	})
	go deepWorker.Run(ctx)

	proxies := make([]*proxy.Server, 0, len(channels))
	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}
		addr := fmt.Sprintf("%s:%d", ch.ListenHost, ch.ListenPort)
		proxies = append(proxies, proxy.NewServer(addr, ch.ID, cfg.ProxyUsername, cfg.ProxyPassword, manager, tracker))
	}

	return services{
		admin: admin.NewServer(manager, nodes, tracker,
			admin.WithConfig(cfgPath, cfg),
			admin.WithStorage(database),
			admin.WithNodeRefresher(func(ctx context.Context) ([]node.Node, error) {
				if err := refreshNodes(ctx); err != nil {
					return nil, err
				}
				return nodes.List(), nil
			}),
			admin.WithNodeChecker(nodecheck.Checker{Timeout: 3 * time.Second}.Check),
			admin.WithRestarter(func(ctx context.Context) error {
				go func() {
					time.Sleep(200 * time.Millisecond)
					process, err := os.FindProcess(os.Getpid())
					if err != nil {
						log.Printf("find process for restart: %v", err)
						os.Exit(0)
						return
					}
					if err := process.Signal(syscall.SIGTERM); err != nil {
						log.Printf("signal process for restart: %v", err)
						os.Exit(0)
					}
				}()
				return nil
			}),
		),
		nodes:      nodes,
		channels:   manager,
		tracker:    tracker,
		storage:    database,
		proxies:    proxies,
		configPath: cfgPath,
	}, nil
}

func loadNodes(ctx context.Context, cfg config.Config) ([]node.Node, error) {
	if cfg.TunnelBackend == config.TunnelBackendFake {
		return demoNodes(), nil
	}
	nodes, err := (vpngate.Client{}).Fetch(ctx)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("vpngate returned no nodes")
	}
	enriched, err := (ipinfo.Client{}).Enrich(ctx, nodes)
	if err != nil {
		log.Printf("ip info enrich failed: %v", err)
		return nodes, nil
	}
	return enriched, nil
}

func startNodeUpdater(ctx context.Context, cfg config.Config, store *node.Store, database *storage.Store) {
	if store == nil {
		return
	}
	interval, err := config.ParseNodeRefreshInterval(cfg.NodeRefreshInterval)
	if err != nil {
		log.Printf("node refresh interval disabled: %v", err)
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				nodes, err := loadNodes(ctx, cfg)
				if err != nil {
					log.Printf("scheduled node update failed: %v", err)
					continue
				}
				store.Replace(nodes)
				if database != nil {
					if err := database.ReplaceNodes(ctx, nodes); err != nil {
						log.Printf("scheduled node cache failed: %v", err)
					}
				}
				log.Printf("scheduled node update loaded %d nodes", len(nodes))
			}
		}
	}()
}

type channelHistoryAdapter struct {
	store *storage.Store
}

func (a channelHistoryAdapter) RecentNodeIDsForChannel(ctx context.Context, channelID string, since time.Time) (map[string]time.Time, error) {
	return a.store.RecentNodeIDsForChannel(ctx, channelID, since)
}

func (a channelHistoryAdapter) DeepTestResults(ctx context.Context) (map[string]deeptest.Result, error) {
	return a.store.DeepTestResults(ctx)
}

func (a channelHistoryAdapter) RecordChannelNodeUse(ctx context.Context, use channel.NodeUse) error {
	return a.store.RecordChannelNodeUse(ctx, storage.ChannelNodeUse{
		ChannelID:   use.ChannelID,
		NodeID:      use.NodeID,
		ExitIP:      use.ExitIP,
		ConnectedAt: use.ConnectedAt,
		SwitchedAt:  use.SwitchedAt,
	})
}

func tunnelFactory(cfg config.Config) channel.TunnelFactory {
	switch cfg.TunnelBackend {
	case config.TunnelBackendOpenVPN:
		return func(name string) tunnel.Tunnel {
			return tunnel.NewOpenVPN(tunnel.OpenVPNConfig{DataDir: cfg.DataDir, Command: cfg.OpenVPNCommand})
		}
	default:
		return func(name string) tunnel.Tunnel {
			return tunnel.NewFake(name)
		}
	}
}

func demoNodes() []node.Node {
	return []node.Node{
		{ID: "jp-demo", Region: "jp", IP: "203.0.113.10", Hostname: "jp-demo", Port: 1194, Proto: "udp", LatencyMS: 50, Speed: 1000, Available: true, IPType: "residential", Quality: "normal", PurityScore: 90, Owner: "Demo Home ISP"},
		{ID: "us-demo", Region: "us", IP: "198.51.100.10", Hostname: "us-demo", Port: 443, Proto: "tcp", LatencyMS: 60, Speed: 900, Available: true, IPType: "hosting", Quality: "datacenter", PurityScore: 45, Owner: "Demo Cloud"},
	}
}
