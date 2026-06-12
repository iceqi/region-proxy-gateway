package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/iceqi/region-proxy-gateway/internal/admin"
	"github.com/iceqi/region-proxy-gateway/internal/config"
	"github.com/iceqi/region-proxy-gateway/internal/connection"
	"github.com/iceqi/region-proxy-gateway/internal/node"
	"github.com/iceqi/region-proxy-gateway/internal/session"
	"github.com/iceqi/region-proxy-gateway/internal/tunnel"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version)
		return
	}

	cfg := config.Default()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("validate config: %v", err)
	}

	adminServer := buildAdminServer(cfg)
	addr := fmt.Sprintf("%s:%d", cfg.AdminHost, cfg.AdminPort)
	log.Printf("admin listening on http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, adminServer))
}

func buildAdminServer(cfg config.Config) *admin.Server {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-demo", Region: "jp", IP: "203.0.113.10", LatencyMS: 50, Available: true},
		{ID: "us-demo", Region: "us", IP: "198.51.100.10", LatencyMS: 60, Available: true},
	})

	factory := func(key string) tunnel.Tunnel {
		return tunnel.NewFake(key)
	}
	sessions := session.NewManager(nodes, cfg.MaxActiveSessions, factory)
	return admin.NewServer(sessions, nodes, connection.NewTracker())
}
