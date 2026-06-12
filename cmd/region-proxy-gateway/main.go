package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/iceqi/region-proxy-gateway/internal/admin"
	"github.com/iceqi/region-proxy-gateway/internal/config"
	"github.com/iceqi/region-proxy-gateway/internal/connection"
	"github.com/iceqi/region-proxy-gateway/internal/node"
	"github.com/iceqi/region-proxy-gateway/internal/proxy"
	"github.com/iceqi/region-proxy-gateway/internal/session"
	"github.com/iceqi/region-proxy-gateway/internal/tunnel"
)

var version = "dev"

type services struct {
	admin    *admin.Server
	proxy    *proxy.Server
	nodes    *node.Store
	sessions *session.Manager
	tracker  *connection.Tracker
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version)
		return
	}

	cfg := config.Default()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("validate config: %v", err)
	}

	services := buildServices(cfg)
	adminAddr := fmt.Sprintf("%s:%d", cfg.AdminHost, cfg.AdminPort)
	proxyAddr := services.proxy.ListenAddr

	proxyListener, err := net.Listen("tcp", proxyAddr)
	if err != nil {
		log.Fatalf("proxy listen on %s: %v", proxyAddr, err)
	}

	go func() {
		if err := services.proxy.Serve(proxyListener); err != nil {
			log.Printf("proxy server stopped: %v", err)
		}
	}()

	log.Printf("proxy listening on %s", proxyAddr)
	log.Printf("admin listening on http://%s", adminAddr)
	log.Fatal(http.ListenAndServe(adminAddr, services.admin))
}

func buildServices(cfg config.Config) services {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-demo", Region: "jp", IP: "203.0.113.10", LatencyMS: 50, Available: true},
		{ID: "us-demo", Region: "us", IP: "198.51.100.10", LatencyMS: 60, Available: true},
	})

	factory := func(key string) tunnel.Tunnel {
		return tunnel.NewFake(key)
	}
	sessions := session.NewManager(nodes, cfg.MaxActiveSessions, factory)
	tracker := connection.NewTracker()
	proxyAddr := fmt.Sprintf("%s:%d", cfg.ProxyHost, cfg.ProxyPort)

	return services{
		admin: admin.NewServer(sessions, nodes, tracker),
		proxy: proxy.NewServer(
			proxyAddr,
			cfg.ProxyPassword,
			cfg.AllowedRegions,
			cfg.AllowedRotateMinutes,
			sessions,
			tracker,
		),
		nodes:    nodes,
		sessions: sessions,
		tracker:  tracker,
	}
}
