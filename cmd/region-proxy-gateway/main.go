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
	"sync"
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
	admin        *admin.Server
	nodes        *node.Store
	channels     *channel.Manager
	tracker      *connection.Tracker
	storage      *storage.Store
	proxyRuntime *proxyRuntime
	configPath   string
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

	adminAddr := fmt.Sprintf("%s:%d", cfg.AdminHost, cfg.AdminPort)
	log.Printf("admin listening on http://%s", adminAddr)
	adminServer := &http.Server{Addr: adminAddr, Handler: services.admin}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = adminServer.Shutdown(shutdownCtx)
		if services.proxyRuntime != nil {
			_ = services.proxyRuntime.Stop(shutdownCtx)
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

	proxyRuntime := newProxyRuntime(cfg.ProxyUsername, cfg.ProxyPassword, manager, tracker)
	if err := proxyRuntime.Sync(ctx, channels); err != nil {
		_ = database.Close()
		return services{}, err
	}
	reloadRuntime := func(reloadCtx context.Context) error {
		nextCfg, err := config.Load(cfgPath)
		if err == nil {
			proxyRuntime.SetCredentials(nextCfg.ProxyUsername, nextCfg.ProxyPassword)
		} else {
			log.Printf("reload config for proxy credentials failed: %v", err)
		}
		nextChannels, err := database.ListChannels(reloadCtx)
		if err != nil {
			return err
		}
		if err := manager.ReplaceChannels(ctx, nextChannels); err != nil {
			return err
		}
		return proxyRuntime.Sync(ctx, nextChannels)
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
			admin.WithRuntimeReloader(reloadRuntime),
		),
		nodes:        nodes,
		channels:     manager,
		tracker:      tracker,
		storage:      database,
		proxyRuntime: proxyRuntime,
		configPath:   cfgPath,
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

type proxyRuntime struct {
	mu            sync.Mutex
	proxyUsername string
	proxyPassword string
	dialer        proxy.Dialer
	tracker       *connection.Tracker
	entries       map[string]*proxyRuntimeEntry
}

type proxyRuntimeEntry struct {
	channel  config.Channel
	server   *proxy.Server
	listener net.Listener
}

func newProxyRuntime(proxyUsername string, proxyPassword string, dialer proxy.Dialer, tracker *connection.Tracker) *proxyRuntime {
	return &proxyRuntime{
		proxyUsername: proxyUsername,
		proxyPassword: proxyPassword,
		dialer:        dialer,
		tracker:       tracker,
		entries:       map[string]*proxyRuntimeEntry{},
	}
}

func (r *proxyRuntime) Sync(ctx context.Context, channels []config.Channel) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	next := make(map[string]config.Channel, len(channels))
	for _, ch := range channels {
		if ch.Enabled {
			next[ch.ID] = ch
		}
	}
	for id, entry := range r.entries {
		ch, ok := next[id]
		if !ok || proxyListenAddr(ch) != proxyListenAddr(entry.channel) {
			_ = entry.listener.Close()
			delete(r.entries, id)
		}
	}
	for id, ch := range next {
		if _, ok := r.entries[id]; ok {
			r.entries[id].channel = ch
			continue
		}
		addr := proxyListenAddr(ch)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("proxy listen on %s: %w", addr, err)
		}
		server := proxy.NewServer(addr, ch.ID, r.proxyUsername, r.proxyPassword, r.dialer, r.tracker)
		entry := &proxyRuntimeEntry{channel: ch, server: server, listener: listener}
		r.entries[id] = entry
		go func(entry *proxyRuntimeEntry) {
			if err := entry.server.Serve(entry.listener); err != nil {
				log.Printf("proxy channel %s stopped: %v", entry.server.ChannelID, err)
			}
		}(entry)
		log.Printf("proxy channel %s listening on %s", ch.ID, addr)
	}
	return nil
}

func (r *proxyRuntime) SetCredentials(username string, password string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.proxyUsername = username
	r.proxyPassword = password
	for _, entry := range r.entries {
		entry.server.SetCredentials(username, password)
	}
}

func (r *proxyRuntime) Stop(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, entry := range r.entries {
		_ = entry.listener.Close()
		delete(r.entries, id)
	}
	return nil
}

func proxyListenAddr(ch config.Channel) string {
	return fmt.Sprintf("%s:%d", ch.ListenHost, ch.ListenPort)
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
