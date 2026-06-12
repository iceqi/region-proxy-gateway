package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/iceqi/region-proxy-gateway/internal/admin"
	"github.com/iceqi/region-proxy-gateway/internal/channel"
	"github.com/iceqi/region-proxy-gateway/internal/config"
	"github.com/iceqi/region-proxy-gateway/internal/connection"
	"github.com/iceqi/region-proxy-gateway/internal/node"
	"github.com/iceqi/region-proxy-gateway/internal/proxy"
	"github.com/iceqi/region-proxy-gateway/internal/tunnel"
	"github.com/iceqi/region-proxy-gateway/internal/vpngate"
)

var version = "dev"

type services struct {
	admin      *admin.Server
	nodes      *node.Store
	channels   *channel.Manager
	tracker    *connection.Tracker
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

	ctx := context.Background()
	services, err := buildServices(ctx, cfg, cfgPath)
	if err != nil {
		log.Fatalf("build services: %v", err)
	}
	defer services.channels.Stop(ctx)

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
	log.Fatal(http.ListenAndServe(adminAddr, services.admin))
}

func buildServices(ctx context.Context, cfg config.Config, cfgPath string) (services, error) {
	if err := cfg.Validate(); err != nil {
		return services{}, err
	}

	nodes := node.NewStore()
	loadedNodes, err := loadNodes(ctx, cfg)
	if err != nil {
		return services{}, err
	}
	nodes.Replace(loadedNodes)

	tracker := connection.NewTracker()
	manager := channel.NewManager(channel.Config{
		Channels:      cfg.Channels,
		Nodes:         nodes,
		TunnelFactory: tunnelFactory(cfg),
		DataDir:       cfg.DataDir,
		OpenVPNCmd:    cfg.OpenVPNCommand,
	})
	if err := manager.Start(ctx); err != nil {
		return services{}, err
	}

	proxies := make([]*proxy.Server, 0, len(cfg.Channels))
	for _, ch := range cfg.Channels {
		if !ch.Enabled {
			continue
		}
		addr := fmt.Sprintf("%s:%d", ch.ListenHost, ch.ListenPort)
		proxies = append(proxies, proxy.NewServer(addr, ch.ID, cfg.ProxyUsername, cfg.ProxyPassword, manager, tracker))
	}

	return services{
		admin:      admin.NewServer(manager, nodes, tracker),
		nodes:      nodes,
		channels:   manager,
		tracker:    tracker,
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
	return nodes, nil
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
		{ID: "jp-demo", Region: "jp", IP: "203.0.113.10", Hostname: "jp-demo", LatencyMS: 50, Speed: 1000, Available: true},
		{ID: "us-demo", Region: "us", IP: "198.51.100.10", Hostname: "us-demo", LatencyMS: 60, Speed: 900, Available: true},
	}
}
