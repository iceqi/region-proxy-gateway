package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/config"
	"github.com/iceqi/region-proxy-gateway/internal/node"
)

func TestLoadNodesFiltersOutUnreachableNodes(t *testing.T) {
	ctx := context.Background()
	nodes := []node.Node{
		{ID: "ok-1", Region: "jp", Hostname: "ok-1", Available: true, OpenVPN: "client\n"},
		{ID: "bad-1", Region: "jp", Hostname: "bad-1", Available: true, OpenVPN: "client\n"},
	}
	filtered, err := filterConnectableNodes(ctx, nodes, nodeConnectivityTesterFunc(func(ctx context.Context, n node.Node) error {
		if n.ID == "bad-1" {
			return fmt.Errorf("dial failed")
		}
		return nil
	}))
	if err != nil {
		t.Fatalf("filterConnectableNodes: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != "ok-1" {
		t.Fatalf("filtered = %+v, want only ok-1", filtered)
	}
}

func TestFreshNodesKeepsOnlyRecentlyValidatedNodes(t *testing.T) {
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	nodes := []node.Node{
		{ID: "fresh", LastTestedAt: now.Add(-9 * time.Minute)},
		{ID: "stale", LastTestedAt: now.Add(-11 * time.Minute)},
		{ID: "never"},
	}

	filtered := freshNodes(nodes, 10*time.Minute, now)

	if len(filtered) != 1 || filtered[0].ID != "fresh" {
		t.Fatalf("fresh nodes = %+v, want only recently validated node", filtered)
	}
}

func TestBuildServicesReportsDemoNodesAndChannelProxy(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.Channels[0].ListenHost = "127.0.0.1"
	cfg.Channels[0].ListenPort = 12345

	services, err := buildServices(context.Background(), cfg, "")
	if err != nil {
		t.Fatalf("buildServices returned error: %v", err)
	}
	defer services.channels.Stop(context.Background())
	defer services.proxyRuntime.Stop(context.Background())
	defer services.gatewayRuntime.Stop(context.Background())
	defer services.extractRuntime.Stop(context.Background())

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, cfg.AdminPath+"/api/status", nil)
	request.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	services.admin.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusOK)
	}

	var body struct {
		OK              bool `json:"ok"`
		ChannelCount    int  `json:"channel_count"`
		NodeCount       int  `json:"node_count"`
		ConnectionCount int  `json:"connection_count"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !body.OK || body.ChannelCount != 1 || body.NodeCount != 2 || body.ConnectionCount != 0 {
		t.Fatalf("unexpected status body: %+v", body)
	}
	wantProxyAddr := fmt.Sprintf("%s:%d", cfg.Channels[0].ListenHost, cfg.Channels[0].ListenPort)
	if services.proxyRuntime == nil || len(services.proxyRuntime.entries) != 1 || services.proxyRuntime.entries["jp-3000"].server.ListenAddr != wantProxyAddr {
		t.Fatalf("proxy runtime = %+v, want one proxy at %s", services.proxyRuntime, wantProxyAddr)
	}
}

func TestAdminChannelSaveHotReloadsProxyRuntime(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.DatabasePath = filepath.Join(dir, "region-proxy-gateway.db")
	cfg.AdminHost = "127.0.0.1"
	cfg.Channels[0].ListenHost = "127.0.0.1"
	cfg.Channels[0].ListenPort = 12347
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	services, err := buildServices(context.Background(), cfg, cfgPath)
	if err != nil {
		t.Fatalf("buildServices returned error: %v", err)
	}
	defer services.channels.Stop(context.Background())
	defer services.proxyRuntime.Stop(context.Background())
	defer services.gatewayRuntime.Stop(context.Background())
	defer services.extractRuntime.Stop(context.Background())
	defer services.storage.Close()

	req := httptest.NewRequest(http.MethodPost, cfg.AdminPath+"/api/channels", strings.NewReader(`{"id":"us-3001","listen_host":"127.0.0.1","listen_port":12348,"region":"us","rotate_minutes":0,"selection_mode":"auto","enabled":true}`))
	req.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	rec := httptest.NewRecorder()

	services.admin.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("save status = %d body=%s", rec.Code, rec.Body.String())
	}
	if _, ok := services.proxyRuntime.entries["us-3001"]; !ok {
		t.Fatalf("proxy runtime entries = %+v, want us-3001 hot loaded", services.proxyRuntime.entries)
	}
	if _, ok := services.channels.Snapshot("us-3001"); !ok {
		t.Fatalf("channel manager should hot load us-3001")
	}
}

func TestExtractAPIActivatesFreshDynamicProxyPortPerCall(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.AdminHost = "127.0.0.1"
	cfg.ProxyUsername = "proxy"
	cfg.ProxyPassword = "secret"
	cfg.ProxyExtractAPIPort = 39002

	services, err := buildServices(context.Background(), cfg, "")
	if err != nil {
		t.Fatalf("buildServices returned error: %v", err)
	}
	defer services.channels.Stop(context.Background())
	defer services.proxyRuntime.Stop(context.Background())
	defer services.gatewayRuntime.Stop(context.Background())
	defer services.extractRuntime.Stop(context.Background())
	defer services.storage.Close()

	var ports []string
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, cfg.AdminPath+"/api/proxies/extract?format=json&count=1", nil)
		req.Host = "203.0.113.99:8787"
		req.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
		rec := httptest.NewRecorder()

		services.admin.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("request %d status = %d body=%s", i+1, rec.Code, rec.Body.String())
		}
		var body struct {
			Proxies []struct {
				HTTP string `json:"http"`
			} `json:"proxies"`
			Cached bool `json:"cached"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode request %d: %v", i+1, err)
		}
		if body.Cached {
			t.Fatalf("request %d cached = true, want dynamic extraction uncached", i+1)
		}
		if len(body.Proxies) != 1 {
			t.Fatalf("request %d proxies = %+v, want one", i+1, body.Proxies)
		}
		parsed, err := url.Parse(body.Proxies[0].HTTP)
		if err != nil {
			t.Fatalf("parse request %d url %q: %v", i+1, body.Proxies[0].HTTP, err)
		}
		ports = append(ports, parsed.Port())
	}

	if ports[0] == "" || ports[1] == "" || ports[0] == ports[1] {
		t.Fatalf("ports = %+v, want two different dynamic ports", ports)
	}
	if ports[0] == strconv.Itoa(cfg.ProxyExtractAPIPort) || ports[1] == strconv.Itoa(cfg.ProxyExtractAPIPort) {
		t.Fatalf("ports = %+v, should not use fixed extract api port %d", ports, cfg.ProxyExtractAPIPort)
	}
}

func TestBuildServicesSharesTrackerBetweenAdminAndProxy(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	services, err := buildServices(context.Background(), cfg, "")
	if err != nil {
		t.Fatalf("buildServices returned error: %v", err)
	}
	defer services.channels.Stop(context.Background())
	defer services.proxyRuntime.Stop(context.Background())
	defer services.gatewayRuntime.Stop(context.Background())
	defer services.extractRuntime.Stop(context.Background())

	services.tracker.Start("127.0.0.1:50000", "http", "jp-3000", "example.com:443")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, cfg.AdminPath+"/api/status", nil)
	request.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	services.admin.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusOK)
	}

	var body struct {
		ConnectionCount int `json:"connection_count"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body.ConnectionCount != 1 {
		t.Fatalf("connection_count = %d, want 1", body.ConnectionCount)
	}
}

func TestBuildServicesMigratesConfigChannelsToSQLite(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.DatabasePath = filepath.Join(dir, "region-proxy-gateway.db")
	cfg.Channels[0].ListenHost = "127.0.0.1"
	cfg.Channels[0].ListenPort = 12346
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	services, err := buildServices(context.Background(), cfg, cfgPath)
	if err != nil {
		t.Fatalf("buildServices returned error: %v", err)
	}
	defer services.channels.Stop(context.Background())
	defer services.proxyRuntime.Stop(context.Background())
	defer services.gatewayRuntime.Stop(context.Background())
	defer services.extractRuntime.Stop(context.Background())
	defer services.storage.Close()

	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load migrated config: %v", err)
	}
	if len(loaded.Channels) != 0 {
		t.Fatalf("config channels = %+v, want cleared after sqlite migration", loaded.Channels)
	}
	channels, err := services.storage.ListChannels(context.Background())
	if err != nil {
		t.Fatalf("sqlite ListChannels: %v", err)
	}
	if len(channels) != 1 || channels[0].ID != "jp-3000" {
		t.Fatalf("sqlite channels = %+v, want migrated jp-3000", channels)
	}
}
