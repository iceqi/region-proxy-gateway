package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/admin"
	"github.com/iceqi/region-proxy-gateway/internal/channel"
	"github.com/iceqi/region-proxy-gateway/internal/config"
	"github.com/iceqi/region-proxy-gateway/internal/node"
	"github.com/iceqi/region-proxy-gateway/internal/tunnel"
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
	cfg.TunnelBackend = config.TunnelBackendFake
	cfg.DataDir = t.TempDir()
	cfg.DatabasePath = filepath.Join(cfg.DataDir, "region-proxy-gateway.db")
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

	if !body.OK || body.ChannelCount != 1 || body.ConnectionCount != 0 {
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
	cfg.TunnelBackend = config.TunnelBackendFake
	cfg.DataDir = t.TempDir()
	cfg.DatabasePath = filepath.Join(cfg.DataDir, "region-proxy-gateway.db")
	cfg.AdminHost = "127.0.0.1"
	cfg.ProxyUsername = "proxy"
	cfg.ProxyPassword = "secret"
	cfg.ProxyExtractAPIPort = 39002
	cfg.Channels[0].ListenHost = "127.0.0.1"
	cfg.Channels[0].ListenPort = 12349

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

func TestDynamicExtractRuntimeSkipsFailedNodeAndKeepsOldPort(t *testing.T) {
	nodes := node.NewStore()
	now := time.Now()
	nodes.Replace([]node.Node{
		{ID: "bad", Region: "jp", IP: "203.0.113.10", Available: true, LastTestedAt: now, OpenVPN: "client\n"},
		{ID: "good", Region: "jp", IP: "203.0.113.11", Available: true, LastTestedAt: now, OpenVPN: "client\n"},
	})
	manager := channel.NewManager(channel.Config{
		Channels: []config.Channel{},
		Nodes:    nodes,
		TunnelFactory: func(name string) tunnel.Tunnel {
			return &failingNodeTunnel{failedNodeID: "bad"}
		},
		DataDir: t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("manager start: %v", err)
	}
	defer manager.Stop(context.Background())
	cfg := config.Default()
	cfg.ProxyExtractAPIHost = "127.0.0.1"
	runtime := newDynamicExtractRuntime(cfg, manager, nil, nodes)

	items, err := runtime.Activate(context.Background(), admin.ProxyExtractRequest{Region: "jp", Host: "203.0.113.99", Count: 1})
	if err != nil {
		t.Fatalf("Activate returned error: %v", err)
	}
	if len(items) != 1 || items[0].CurrentExitIP != "203.0.113.11" {
		t.Fatalf("items = %+v, want good node selected after bad node start failure", items)
	}
	bad, _ := nodes.NodeByID("bad")
	if bad.Available || bad.ProbeStatus != "unavailable" {
		t.Fatalf("bad node after failure = %+v, want marked unavailable", bad)
	}
}

func TestDynamicExtractRuntimeKeepsOldPortWhenReplacementFails(t *testing.T) {
	nodes := node.NewStore()
	now := time.Now()
	nodes.Replace([]node.Node{{ID: "good", Region: "jp", IP: "203.0.113.11", Available: true, LastTestedAt: now, OpenVPN: "client\n"}})
	failedNodeID := ""
	manager := channel.NewManager(channel.Config{
		Channels: []config.Channel{},
		Nodes:    nodes,
		TunnelFactory: func(name string) tunnel.Tunnel {
			return &failingNodeTunnel{failedNodeID: failedNodeID}
		},
		DataDir: t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("manager start: %v", err)
	}
	defer manager.Stop(context.Background())
	cfg := config.Default()
	cfg.ProxyExtractAPIHost = "127.0.0.1"
	runtime := newDynamicExtractRuntime(cfg, manager, nil, nodes)

	first, err := runtime.Activate(context.Background(), admin.ProxyExtractRequest{Region: "jp", Host: "203.0.113.99", Count: 1})
	if err != nil {
		t.Fatalf("first Activate returned error: %v", err)
	}
	oldChannelID := runtime.current.channelID

	nodes.Replace([]node.Node{{ID: "bad", Region: "jp", IP: "203.0.113.12", Available: true, LastTestedAt: now, OpenVPN: "client\n"}})
	failedNodeID = "bad"

	_, err = runtime.Activate(context.Background(), admin.ProxyExtractRequest{Region: "jp", Host: "203.0.113.99", Count: 1})
	if err == nil {
		t.Fatalf("second Activate returned nil error, want failure")
	}
	if runtime.current == nil || runtime.current.channelID != oldChannelID {
		t.Fatalf("current entry changed after failed replacement: got %+v want channel %s from first %+v", runtime.current, oldChannelID, first)
	}
	if _, ok := manager.Snapshot(oldChannelID); !ok {
		t.Fatalf("old channel %q was removed after failed replacement", oldChannelID)
	}
}

func TestBuildServicesSharesTrackerBetweenAdminAndProxy(t *testing.T) {
	cfg := config.Default()
	cfg.TunnelBackend = config.TunnelBackendFake
	cfg.DataDir = t.TempDir()
	cfg.DatabasePath = filepath.Join(cfg.DataDir, "region-proxy-gateway.db")
	cfg.Channels[0].ListenHost = "127.0.0.1"
	cfg.Channels[0].ListenPort = 12350
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

type failingNodeTunnel struct {
	failedNodeID string
	current      node.Node
	status       tunnel.Status
}

func (t *failingNodeTunnel) Start(ctx context.Context, n node.Node, opts tunnel.Options) error {
	if n.ID == t.failedNodeID {
		return fmt.Errorf("openvpn process exited before device became ready")
	}
	t.current = n
	t.status = tunnel.Status{Name: opts.Name, NodeID: n.ID, Ready: true, StartedAt: time.Now()}
	return nil
}

func (t *failingNodeTunnel) Stop(ctx context.Context) error {
	t.status.Ready = false
	return nil
}

func (t *failingNodeTunnel) Switch(ctx context.Context, n node.Node) error {
	t.current = n
	t.status.NodeID = n.ID
	t.status.Ready = true
	return nil
}

func (t *failingNodeTunnel) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	client, server := net.Pipe()
	_ = server.Close()
	return client, nil
}

func (t *failingNodeTunnel) Status() tunnel.Status {
	return t.status
}
