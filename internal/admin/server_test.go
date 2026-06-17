package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/channel"
	"github.com/iceqi/region-proxy-gateway/internal/config"
	"github.com/iceqi/region-proxy-gateway/internal/connection"
	"github.com/iceqi/region-proxy-gateway/internal/deeptest"
	"github.com/iceqi/region-proxy-gateway/internal/node"
	"github.com/iceqi/region-proxy-gateway/internal/storage"
	"github.com/iceqi/region-proxy-gateway/internal/tunnel"
)

func TestStatusReturnsChannelsAndConnections(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	connections := connection.NewTracker()
	connections.Start("127.0.0.1:12345", "http", "jp-3000", "example.com:443")
	server := NewServer(manager, nodes, connections)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/status", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body struct {
		OK              bool `json:"ok"`
		ChannelCount    int  `json:"channel_count"`
		NodeCount       int  `json:"node_count"`
		ConnectionCount int  `json:"connection_count"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.OK || body.ChannelCount != 1 || body.ConnectionCount != 1 {
		t.Fatalf("unexpected status body: %+v", body)
	}
}

func TestChannelsReturnsSnapshots(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	cfg := config.Default()
	cfg.ProxyUsername = "alice"
	cfg.ProxyPassword = "secret"
	server := NewServer(manager, nodes, nil, WithConfig("", cfg))

	req := httptest.NewRequest(http.MethodGet, "/admin/api/channels", nil)
	req.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var body struct {
		Channels []channelView `json:"channels"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Channels) != 1 {
		t.Fatalf("channels = %d, want 1", len(body.Channels))
	}
	if body.Channels[0].ID != "jp-3000" {
		t.Fatalf("channel id = %q, want jp-3000", body.Channels[0].ID)
	}
	if body.Channels[0].ProxyAuthHTTP != "http://alice:secret@0.0.0.0:3000" {
		t.Fatalf("auth http = %q", body.Channels[0].ProxyAuthHTTP)
	}
	if body.Channels[0].CurrentNode.IP == "" && body.Channels[0].CurrentNode.Hostname == "" {
		t.Fatalf("channel current node should include exit address: %+v", body.Channels[0].CurrentNode)
	}
}

func TestExtractProxiesRequiresAdminAuth(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	cfg := config.Default()
	server := NewServer(manager, nodes, nil, WithConfig("", cfg))

	req := httptest.NewRequest(http.MethodGet, "/admin/api/proxies/extract", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want unauthorized", rec.Code)
	}
}

func TestExtractProxiesCanUseQueryToken(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	cfg := config.Default()
	cfg.ProxyExtractAPIToken = "extract-token"
	server := NewServer(manager, nodes, nil, WithConfig("", cfg), WithProxyExtractorValidator(func(ctx context.Context, proxy proxyExtractItem) error {
		return nil
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/api/proxies/extract?token=extract-token&format=text&count=1", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want ok with token", rec.Code, rec.Body.String())
	}
}

func TestExtractProxiesRejectsWrongQueryToken(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	cfg := config.Default()
	cfg.ProxyExtractAPIToken = "extract-token"
	server := NewServer(manager, nodes, nil, WithConfig("", cfg), WithProxyExtractorValidator(func(ctx context.Context, proxy proxyExtractItem) error {
		return nil
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/api/proxies/extract?token=wrong&format=text&count=1", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s, want unauthorized", rec.Code, rec.Body.String())
	}
}

func TestExtractProxiesReturnsJSONAndText(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	cfg := config.Default()
	cfg.ProxyUsername = "proxy"
	cfg.ProxyPassword = "secret"
	server := NewServer(manager, nodes, nil, WithConfig("", cfg), WithProxyExtractorValidator(func(ctx context.Context, proxy proxyExtractItem) error {
		return nil
	}))

	jsonReq := httptest.NewRequest(http.MethodGet, "/admin/api/proxies/extract?format=json&scheme=socks5&count=1", nil)
	jsonReq.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	jsonRec := httptest.NewRecorder()

	server.ServeHTTP(jsonRec, jsonReq)

	if jsonRec.Code != http.StatusOK {
		t.Fatalf("json status = %d body=%s", jsonRec.Code, jsonRec.Body.String())
	}
	var body struct {
		Proxies []proxyExtractItem `json:"proxies"`
	}
	if err := json.NewDecoder(jsonRec.Body).Decode(&body); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if len(body.Proxies) != 1 || body.Proxies[0].SOCKS5 == "" || !strings.Contains(body.Proxies[0].SOCKS5, "proxy:secret@") {
		t.Fatalf("json body = %+v, want one auth socks5 proxy", body)
	}

	textReq := httptest.NewRequest(http.MethodGet, "/admin/api/proxies/extract?format=text&scheme=no-scheme&count=1", nil)
	textReq.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	textRec := httptest.NewRecorder()

	server.ServeHTTP(textRec, textReq)

	if textRec.Code != http.StatusOK {
		t.Fatalf("text status = %d body=%s", textRec.Code, textRec.Body.String())
	}
	if got := strings.TrimSpace(textRec.Body.String()); !strings.Contains(got, "proxy:secret@") || strings.Contains(got, "://") {
		t.Fatalf("text body = %q, want no-scheme auth proxy", got)
	}
}

func TestExtractProxiesSkipsValidationFailures(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	cfg := config.Default()
	cfg.ProxyUsername = "proxy"
	cfg.ProxyPassword = "secret"
	server := NewServer(manager, nodes, nil, WithConfig("", cfg), WithProxyExtractorValidator(func(ctx context.Context, proxy proxyExtractItem) error {
		return context.DeadlineExceeded
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/api/proxies/extract?format=json&count=1", nil)
	req.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body=%s, want bad gateway", rec.Code, rec.Body.String())
	}
}

func TestExtractProxiesRotateParameterBypassesCacheAndSwitchesNode(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-1", Region: "jp", IP: "203.0.113.10", Hostname: "jp-one", Available: true},
		{ID: "jp-2", Region: "jp", IP: "203.0.113.11", Hostname: "jp-two", Available: true},
	})
	manager := channel.NewManager(channel.Config{
		Channels: []config.Channel{{
			ID:            "jp-3000",
			ListenHost:    "127.0.0.1",
			ListenPort:    3000,
			Region:        "jp",
			SelectionMode: config.SelectionAuto,
			Enabled:       true,
		}},
		Nodes: nodes,
		TunnelFactory: func(name string) tunnel.Tunnel {
			return tunnel.NewFake(name)
		},
		DataDir: t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("manager start: %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background()) })
	cfg := config.Default()
	cfg.ProxyExtractCacheTTL = "30s"
	calls := 0
	server := NewServer(manager, nodes, nil, WithConfig("", cfg), WithProxyExtractorValidator(func(ctx context.Context, proxy proxyExtractItem) error {
		calls++
		return nil
	}))

	var exits []string

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/admin/api/proxies/extract?format=json&count=1&rotate=1", nil)
		req.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
		rec := httptest.NewRecorder()

		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("request %d status = %d body=%s", i+1, rec.Code, rec.Body.String())
		}
		var body struct {
			Proxies []proxyExtractItem `json:"proxies"`
			Cached  bool               `json:"cached"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode request %d: %v", i+1, err)
		}
		if body.Cached {
			t.Fatalf("request %d cached = true, want rotate request to bypass cache", i+1)
		}
		if len(body.Proxies) != 1 {
			t.Fatalf("request %d proxies = %+v, want one", i+1, body.Proxies)
		}
		exits = append(exits, body.Proxies[0].CurrentExitIP)
	}
	if calls != 2 {
		t.Fatalf("validator calls = %d, want rotate requests revalidated", calls)
	}
	if exits[0] == exits[1] {
		t.Fatalf("exit IPs = %+v, want rotate parameter to switch node", exits)
	}
}

func TestNodesCanFilterByRegion(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	nodes.Replace(append(nodes.List(), node.Node{ID: "us-1", Region: "us", Available: true}))
	server := NewServer(manager, nodes, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/nodes?region=jp", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var body struct {
		Nodes []node.Node `json:"nodes"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Nodes) != 1 || body.Nodes[0].Region != "jp" {
		t.Fatalf("unexpected nodes: %+v", body.Nodes)
	}
}

func TestNodesAPIUsesCompactViewWithoutOpenVPNConfig(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	nodes.Replace([]node.Node{{
		ID:        "jp-heavy",
		Region:    "jp",
		Country:   "Japan",
		IP:        "203.0.113.88",
		Hostname:  "vpn-heavy",
		Port:      1194,
		Proto:     "udp",
		OpenVPN:   strings.Repeat("client\nremote example.com 1194\n", 200),
		Available: true,
	}})
	server := NewServer(manager, nodes, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/nodes", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "openvpn") || strings.Contains(rec.Body.String(), "remote example.com") {
		t.Fatalf("nodes response should not include raw openvpn config: %s", rec.Body.String())
	}
	var body struct {
		Nodes []nodeView `json:"nodes"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Nodes) != 1 || body.Nodes[0].ID != "jp-heavy" || body.Nodes[0].IP != "203.0.113.88" {
		t.Fatalf("nodes = %+v, want compact node metadata", body.Nodes)
	}
}

func TestRefreshNodesReplacesNodeStore(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	server := NewServer(manager, nodes, nil, WithNodeRefresher(func(ctx context.Context) ([]node.Node, error) {
		return []node.Node{{ID: "us-new", Region: "us", OpenVPN: "client\nremote hidden.example 1194\n", Available: true}}, nil
	}))

	req := httptest.NewRequest(http.MethodPost, "/admin/api/nodes/refresh", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	got := nodes.List()
	if len(got) != 1 || got[0].ID != "us-new" {
		t.Fatalf("nodes = %+v, want us-new", got)
	}
	if strings.Contains(rec.Body.String(), "openvpn") || strings.Contains(rec.Body.String(), "hidden.example") {
		t.Fatalf("refresh response should not include raw openvpn config: %s", rec.Body.String())
	}
}

func TestProbeNodeUpdatesNodeStore(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	server := NewServer(manager, nodes, nil, WithNodeChecker(func(ctx context.Context, n node.Node) node.Node {
		n.LatencyMS = 33
		n.Available = true
		n.ProbeStatus = "available"
		return n
	}))

	req := httptest.NewRequest(http.MethodPost, "/admin/api/nodes/jp-1/probe", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	got := nodes.List()[0]
	if got.LatencyMS != 33 || got.ProbeStatus != "available" {
		t.Fatalf("node not updated after probe: %+v", got)
	}
}

func TestProbeNodesUpdatesSelectedNodes(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	nodes.Replace([]node.Node{
		{ID: "jp-1", Region: "jp", Available: true},
		{ID: "jp-2", Region: "jp", Available: true},
		{ID: "us-1", Region: "us", Available: true},
	})
	server := NewServer(manager, nodes, nil, WithNodeChecker(func(ctx context.Context, n node.Node) node.Node {
		if n.ID == "jp-1" {
			n.LatencyMS = 21
		} else {
			n.LatencyMS = 34
		}
		n.Available = true
		n.ProbeStatus = "available"
		return n
	}))

	req := httptest.NewRequest(http.MethodPost, "/admin/api/nodes/probe-batch", bytes.NewBufferString(`{"node_ids":["jp-1","jp-2"]}`))
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Count int         `json:"count"`
		Nodes []node.Node `json:"nodes"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Count != 2 || len(body.Nodes) != 2 {
		t.Fatalf("body = %+v, want 2 checked nodes", body)
	}
	got := nodes.List()
	latencyByID := map[string]int{}
	for _, n := range got {
		latencyByID[n.ID] = n.LatencyMS
	}
	if latencyByID["jp-1"] != 21 || latencyByID["jp-2"] != 34 {
		t.Fatalf("latencies = %+v, want selected jp nodes updated", latencyByID)
	}
	if latencyByID["us-1"] != 0 {
		t.Fatalf("us-1 latency = %d, want untouched", latencyByID["us-1"])
	}
}

func TestDeepTestEnqueueAndStatusEndpoints(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	store := openAdminTestStore(t)
	server := NewServer(manager, nodes, nil, WithStorage(store))

	req := httptest.NewRequest(http.MethodPost, "/admin/api/deep-tests", bytes.NewBufferString(`{"node_ids":["jp-1","jp-1"]}`))
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var enqueueBody struct {
		Summary deeptest.EnqueueSummary `json:"summary"`
		Stats   deeptest.QueueStats     `json:"stats"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&enqueueBody); err != nil {
		t.Fatalf("decode enqueue response: %v", err)
	}
	if enqueueBody.Summary.Created != 1 || enqueueBody.Summary.Skipped != 1 || enqueueBody.Stats.Pending != 1 {
		t.Fatalf("enqueue body = %+v, want 1 created 1 skipped 1 pending", enqueueBody)
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/admin/api/deep-tests/status", nil)
	statusRec := httptest.NewRecorder()

	server.ServeHTTP(statusRec, statusReq)

	if statusRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", statusRec.Code, statusRec.Body.String())
	}
	var statusBody struct {
		Stats deeptest.QueueStats `json:"stats"`
	}
	if err := json.NewDecoder(statusRec.Body).Decode(&statusBody); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if statusBody.Stats.Pending != 1 {
		t.Fatalf("status body = %+v, want pending 1", statusBody)
	}
}

func TestNodesAPIIncludesDeepTestResult(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	store := openAdminTestStore(t)
	if _, err := store.EnqueueDeepTestJobs(context.Background(), []string{"jp-1"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	jobs, err := store.ClaimDeepTestJobs(context.Background(), 1, time.Now())
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := store.CompleteDeepTestJob(context.Background(), jobs[0].ID, deeptest.Result{
		NodeID:      "jp-1",
		Status:      deeptest.StatusSuccess,
		ExitIP:      "203.0.113.99",
		ExitCountry: "Japan",
		ConnectMS:   456,
		TestedAt:    time.Now(),
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	server := NewServer(manager, nodes, nil, WithStorage(store))

	req := httptest.NewRequest(http.MethodGet, "/admin/api/nodes", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Nodes []nodeView `json:"nodes"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Nodes) != 1 || body.Nodes[0].DeepTest == nil || body.Nodes[0].DeepTest.ExitIP != "203.0.113.99" {
		t.Fatalf("nodes = %+v, want deep test result", body.Nodes)
	}
}

func TestSettingsCanUpdateNodeRefreshInterval(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	server := NewServer(manager, nodes, nil, WithConfig(path, cfg))

	req := httptest.NewRequest(http.MethodPost, "/admin/api/settings", bytes.NewBufferString(`{"node_refresh_interval":"7m"}`))
	req.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.NodeRefreshInterval != "7m" {
		t.Fatalf("node refresh interval = %q, want 7m", loaded.NodeRefreshInterval)
	}
}

func TestSettingsCanUpdateProxyExtractCacheTTL(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	server := NewServer(manager, nodes, nil, WithConfig(path, cfg))

	req := httptest.NewRequest(http.MethodPost, "/admin/api/settings", bytes.NewBufferString(`{"node_refresh_interval":"20m","proxy_extract_cache_ttl":"45s","proxy_extract_api_token":"new-token"}`))
	req.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.ProxyExtractCacheTTL != "45s" {
		t.Fatalf("proxy extract cache ttl = %q, want 45s", loaded.ProxyExtractCacheTTL)
	}
	if loaded.ProxyExtractAPIToken != "new-token" {
		t.Fatalf("proxy extract api token = %q, want new-token", loaded.ProxyExtractAPIToken)
	}
}

func TestSettingsCanGenerateProxyExtractAPIToken(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	cfg.ProxyExtractAPIToken = "old-token"
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	server := NewServer(manager, nodes, nil, WithConfig(path, cfg))

	req := httptest.NewRequest(http.MethodPost, "/admin/api/settings/proxy-extract-token", nil)
	req.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Settings settingsView `json:"settings"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Settings.ProxyExtractAPIToken == "" || body.Settings.ProxyExtractAPIToken == "old-token" {
		t.Fatalf("generated token = %q, want new non-empty token", body.Settings.ProxyExtractAPIToken)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.ProxyExtractAPIToken != body.Settings.ProxyExtractAPIToken {
		t.Fatalf("persisted token = %q, want %q", loaded.ProxyExtractAPIToken, body.Settings.ProxyExtractAPIToken)
	}
}

func TestSettingsCanUpdateAccessAndProxyCredentials(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	cfg := config.Default()
	cfg.AdminPath = "/admin"
	cfg.AdminUsername = "admin"
	cfg.AdminPassword = "old-admin"
	cfg.ProxyUsername = "proxy"
	cfg.ProxyPassword = "old-proxy"
	cfg.Channels = nil
	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	reloaded := false
	server := NewServer(manager, nodes, nil, WithConfig(path, cfg), WithRuntimeReloader(func(ctx context.Context) error {
		reloaded = true
		return nil
	}))

	req := httptest.NewRequest(http.MethodPost, "/admin/api/settings", bytes.NewBufferString(`{
		"node_refresh_interval":"7m",
		"admin_path":"/secret-admin",
		"admin_username":"root",
		"admin_password":"new-admin",
		"proxy_username":"edge",
		"proxy_password":"new-proxy"
	}`))
	req.SetBasicAuth("admin", "old-admin")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !reloaded {
		t.Fatalf("runtime reloader was not called")
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.AdminPath != "/secret-admin" || loaded.AdminUsername != "root" || loaded.AdminPassword != "new-admin" {
		t.Fatalf("admin config = %+v, want updated credentials/path", loaded)
	}
	if loaded.ProxyUsername != "edge" || loaded.ProxyPassword != "new-proxy" {
		t.Fatalf("proxy credentials = %q/%q, want updated", loaded.ProxyUsername, loaded.ProxyPassword)
	}
	if body := rec.Body.String(); strings.Contains(body, "new-admin") || strings.Contains(body, "new-proxy") {
		t.Fatalf("response leaked password: %s", body)
	}

	newReq := httptest.NewRequest(http.MethodGet, "/secret-admin/api/settings", nil)
	newReq.SetBasicAuth("root", "new-admin")
	newRec := httptest.NewRecorder()
	server.ServeHTTP(newRec, newReq)
	if newRec.Code != http.StatusOK {
		t.Fatalf("new admin path/auth status = %d body=%s", newRec.Code, newRec.Body.String())
	}
}

func TestSettingsEmptyPasswordsKeepExistingSecrets(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	cfg := config.Default()
	cfg.AdminPassword = "old-admin"
	cfg.ProxyPassword = "old-proxy"
	cfg.Channels = nil
	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	server := NewServer(manager, nodes, nil, WithConfig(path, cfg))

	req := httptest.NewRequest(http.MethodPost, "/admin/api/settings", bytes.NewBufferString(`{"node_refresh_interval":"9m","admin_password":"","proxy_password":""}`))
	req.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.AdminPassword != "old-admin" || loaded.ProxyPassword != "old-proxy" {
		t.Fatalf("passwords were changed: admin=%q proxy=%q", loaded.AdminPassword, loaded.ProxyPassword)
	}
}

func TestSettingsRejectsRootAdminPath(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	cfg := config.Default()
	cfg.Channels = nil
	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	server := NewServer(manager, nodes, nil, WithConfig(path, cfg))

	req := httptest.NewRequest(http.MethodPost, "/admin/api/settings", bytes.NewBufferString(`{"node_refresh_interval":"9m","admin_path":"/"}`))
	req.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRestartEndpointCallsRestarter(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	called := false
	server := NewServer(manager, nodes, nil, WithRestarter(func(ctx context.Context) error {
		called = true
		return nil
	}))

	req := httptest.NewRequest(http.MethodPost, "/admin/api/system/restart", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !called {
		t.Fatalf("restarter was not called")
	}
}

func TestIndexReturnsHTML(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	server := NewServer(manager, nodes, nil, WithAdminPath("/secret-admin"))

	req := httptest.NewRequest(http.MethodGet, "/secret-admin/", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q, want html", got)
	}
	if !strings.Contains(rec.Body.String(), "window.location.hostname") {
		t.Fatalf("admin html should generate proxy addresses from browser host")
	}
	for _, plugin := range []string{
		"plugin/advancedFormat.js",
		"plugin/customParseFormat.js",
		"plugin/localeData.js",
		"plugin/quarterOfYear.js",
		"plugin/weekOfYear.js",
		"plugin/weekYear.js",
		"plugin/weekday.js",
	} {
		if !strings.Contains(rec.Body.String(), plugin) {
			t.Fatalf("admin html missing dayjs plugin %s", plugin)
		}
	}
	for _, text := range []string{"regionText", "日本", "自动优选", "Ping 正常"} {
		if !strings.Contains(rec.Body.String(), text) {
			t.Fatalf("admin html missing localized text/helper %q", text)
		}
	}
	if !strings.Contains(rec.Body.String(), "重启服务") || !strings.Contains(rec.Body.String(), "system/restart") {
		t.Fatalf("admin html should include service restart button")
	}
	for _, text := range []string{"content-panel", "text-overflow: ellipsis", "title: value", "测试当前列表延迟", "nodes/probe-batch", "深度测试当前列表", "deep-tests/status", "出口 IP", "channelExitAddress", "normalizeRegion", "候选通道", "matchChannelRegion", "tickNow", "秒", "NO-SCHEME", "proxyAddressNoScheme", "hero-strip", "signal-dot", "login-card", "login-stage", "login-orbit", "login-metrics", "module-card", "section-icon", "module-chip", "table-shell", "settings-grid", "settings-save", "rememberCredentials", "adminAuth", "apiBase + 'proxies/extract", "rotate=1", "旋转网关", "rotate_on_dial", "每次新连接轮换", "旋转网关代理", "rotatingGatewayChannels", "固定入口，每次新连接自动轮换出口 IP"} {
		if !strings.Contains(rec.Body.String(), text) {
			t.Fatalf("admin html missing layout safeguard %q", text)
		}
	}
	if strings.Contains(rec.Body.String(), "apiBase + '/proxies/extract") {
		t.Fatalf("admin html should not generate api//proxies extract URLs")
	}
	if !strings.Contains(rec.Body.String(), "date.getFullYear() <= 1") {
		t.Fatalf("admin html should hide Go zero time values")
	}
}

func TestIndexRedirectsToTrailingSlash(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	server := NewServer(manager, nodes, nil, WithAdminPath("/secret-admin"))

	req := httptest.NewRequest(http.MethodGet, "/secret-admin", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want 301", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/secret-admin/" {
		t.Fatalf("location = %q, want /secret-admin/", got)
	}
}

func TestAdminPathHidesRootAndScopesAPI(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	server := NewServer(manager, nodes, nil, WithAdminPath("/secret-admin"))

	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootRec := httptest.NewRecorder()
	server.ServeHTTP(rootRec, rootReq)
	if rootRec.Code != http.StatusNotFound {
		t.Fatalf("root status = %d, want 404", rootRec.Code)
	}

	apiReq := httptest.NewRequest(http.MethodGet, "/secret-admin/api/nodes", nil)
	apiRec := httptest.NewRecorder()
	server.ServeHTTP(apiRec, apiReq)
	if apiRec.Code != http.StatusOK {
		t.Fatalf("scoped api status = %d, want 200", apiRec.Code)
	}
}

func TestAdminAuthProtectsPanelAndAPI(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	server := NewServer(manager, nodes, nil, WithAdminAuth("admin", "secret"))

	panelReq := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	panelRec := httptest.NewRecorder()
	server.ServeHTTP(panelRec, panelReq)
	if panelRec.Code != http.StatusOK {
		t.Fatalf("panel status without auth = %d, want 200 login page shell", panelRec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/api/nodes", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status without auth = %d, want 401", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") != "" {
		t.Fatalf("unauthorized API should not trigger browser basic auth dialog")
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/api/nodes", nil)
	req.SetBasicAuth("admin", "secret")
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status with auth = %d, want 200", rec.Code)
	}
}

func TestSwitchChannelToNode(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	nodes.Replace(append(nodes.List(), node.Node{ID: "jp-2", Region: "jp", IP: "203.0.113.22", Available: true}))
	server := NewServer(manager, nodes, nil)

	req := httptest.NewRequest(http.MethodPost, "/admin/api/channels/jp-3000/switch", bytes.NewBufferString(`{"node_id":"jp-2"}`))
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	snapshot, ok := manager.Snapshot("jp-3000")
	if !ok {
		t.Fatalf("missing channel snapshot")
	}
	if snapshot.CurrentNodeID != "jp-2" {
		t.Fatalf("current node = %q, want jp-2", snapshot.CurrentNodeID)
	}
}

func TestSwitchChannelToNodeDoesNotRestartService(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	nodes.Replace(append(nodes.List(), node.Node{ID: "jp-2", Region: "jp", IP: "203.0.113.22", Available: true}))
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	cfg.Channels = []config.Channel{{
		ID:            "jp-3000",
		ListenHost:    "127.0.0.1",
		ListenPort:    3000,
		Region:        "jp",
		SelectionMode: config.SelectionAuto,
		Enabled:       true,
	}}
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	restarted := false
	server := NewServer(manager, nodes, nil, WithConfig(path, cfg), WithRestarter(func(ctx context.Context) error {
		restarted = true
		return nil
	}))

	req := httptest.NewRequest(http.MethodPost, "/admin/api/channels/jp-3000/switch", bytes.NewBufferString(`{"node_id":"jp-2"}`))
	req.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if restarted {
		t.Fatalf("switching node should not restart whole service")
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(loaded.Channels) != 1 || loaded.Channels[0].SelectionMode != config.SelectionManual || loaded.Channels[0].ManualNodeID != "jp-2" {
		t.Fatalf("channel config after switch = %+v, want manual jp-2", loaded.Channels)
	}
}

func TestCreateChannelPersistsConfig(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	cfg.Channels = []config.Channel{{
		ID:            "jp-3000",
		ListenHost:    "127.0.0.1",
		ListenPort:    3000,
		Region:        "jp",
		SelectionMode: config.SelectionAuto,
		Enabled:       true,
	}}
	server := NewServer(manager, nodes, nil, WithConfig(path, cfg))

	req := httptest.NewRequest(http.MethodPost, "/admin/api/channels", bytes.NewBufferString(`{"id":"us-3001","listen_host":"0.0.0.0","listen_port":3001,"region":"us","rotate_minutes":0,"selection_mode":"auto","enabled":true}`))
	req.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("load saved config: %v", err)
	}
	if len(loaded.Channels) != 2 {
		t.Fatalf("channels = %d, want 2", len(loaded.Channels))
	}
	if loaded.Channels[1].ID != "us-3001" {
		t.Fatalf("new channel id = %q, want us-3001", loaded.Channels[1].ID)
	}

	channelsReq := httptest.NewRequest(http.MethodGet, "/admin/api/channels", nil)
	channelsReq.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	channelsRec := httptest.NewRecorder()
	server.ServeHTTP(channelsRec, channelsReq)
	if channelsRec.Code != http.StatusOK {
		t.Fatalf("channels status = %d body=%s", channelsRec.Code, channelsRec.Body.String())
	}
	var body struct {
		Channels []channelView `json:"channels"`
	}
	if err := json.NewDecoder(channelsRec.Body).Decode(&body); err != nil {
		t.Fatalf("decode channels: %v", err)
	}
	if len(body.Channels) != 2 || body.Channels[1].ID != "us-3001" {
		t.Fatalf("channels view = %+v, want newly configured channel visible before restart", body.Channels)
	}
}

func TestSaveChannelReloadsRuntimeWithoutRestartingService(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	restarted := false
	reloaded := false
	server := NewServer(manager, nodes, nil, WithConfig(path, cfg), WithRestarter(func(ctx context.Context) error {
		restarted = true
		return nil
	}), WithRuntimeReloader(func(ctx context.Context) error {
		reloaded = true
		return nil
	}))

	req := httptest.NewRequest(http.MethodPost, "/admin/api/channels", bytes.NewBufferString(`{"id":"us-3001","listen_host":"0.0.0.0","listen_port":3001,"region":"us","rotate_minutes":0,"selection_mode":"auto","enabled":true}`))
	req.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if restarted {
		t.Fatalf("saving channel should not restart whole service")
	}
	if !reloaded {
		t.Fatalf("saving channel should reload runtime")
	}
}

func TestUpdateChannelCanRenameExistingConfig(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	cfg.Channels = []config.Channel{{
		ID:            "jp-3000",
		ListenHost:    "127.0.0.1",
		ListenPort:    3000,
		Region:        "jp",
		SelectionMode: config.SelectionAuto,
		Enabled:       true,
	}}
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save initial config: %v", err)
	}
	server := NewServer(manager, nodes, nil, WithConfig(path, cfg))

	req := httptest.NewRequest(http.MethodPost, "/admin/api/channels", bytes.NewBufferString(`{"original_id":"jp-3000","id":"jp-main","listen_host":"0.0.0.0","listen_port":3000,"region":"jp","rotate_minutes":5,"selection_mode":"auto","enabled":true}`))
	req.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("load saved config: %v", err)
	}
	if len(loaded.Channels) != 1 {
		t.Fatalf("channels = %d, want 1", len(loaded.Channels))
	}
	if loaded.Channels[0].ID != "jp-main" || loaded.Channels[0].RotateMinutes != 5 {
		t.Fatalf("updated channel = %+v, want renamed jp-main with rotate 5", loaded.Channels[0])
	}

	channelsReq := httptest.NewRequest(http.MethodGet, "/admin/api/channels", nil)
	channelsReq.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	channelsRec := httptest.NewRecorder()
	server.ServeHTTP(channelsRec, channelsReq)
	if channelsRec.Code != http.StatusOK {
		t.Fatalf("channels status = %d body=%s", channelsRec.Code, channelsRec.Body.String())
	}
	var body struct {
		Channels []channelView `json:"channels"`
	}
	if err := json.NewDecoder(channelsRec.Body).Decode(&body); err != nil {
		t.Fatalf("decode channels: %v", err)
	}
	if len(body.Channels) != 1 || body.Channels[0].ID != "jp-main" || body.Channels[0].ListenHost != "0.0.0.0" {
		t.Fatalf("channels view = %+v, want renamed channel visible before restart", body.Channels)
	}
}

func TestChannelChangesUseSQLiteStorageWhenConfigured(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	cfg.Channels = nil
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	store := openAdminTestStore(t)
	if err := store.SaveChannel(context.Background(), "", config.Channel{ID: "jp-3000", ListenHost: "0.0.0.0", ListenPort: 3000, Region: "jp", SelectionMode: config.SelectionAuto, Enabled: true}); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	server := NewServer(manager, nodes, nil, WithConfig(path, cfg), WithStorage(store))

	req := httptest.NewRequest(http.MethodPost, "/admin/api/channels", bytes.NewBufferString(`{"original_id":"jp-3000","id":"jp-main","listen_host":"0.0.0.0","listen_port":3000,"region":"jp","rotate_minutes":7,"selection_mode":"auto","enabled":true}`))
	req.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save status = %d body=%s", rec.Code, rec.Body.String())
	}

	channels, err := store.ListChannels(context.Background())
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(channels) != 1 || channels[0].ID != "jp-main" || channels[0].RotateMinutes != 7 {
		t.Fatalf("sqlite channels = %+v, want renamed jp-main", channels)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(loaded.Channels) != 0 {
		t.Fatalf("config channels = %+v, want sqlite to own channels", loaded.Channels)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/admin/api/channels/jp-main", nil)
	deleteReq.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	deleteRec := httptest.NewRecorder()
	server.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status = %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
	channels, err = store.ListChannels(context.Background())
	if err != nil {
		t.Fatalf("ListChannels after delete: %v", err)
	}
	if len(channels) != 0 {
		t.Fatalf("sqlite channels = %+v, want empty", channels)
	}
}

func TestDeleteChannelPersistsConfig(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	cfg.Channels = []config.Channel{
		{ID: "jp-3000", ListenHost: "127.0.0.1", ListenPort: 3000, Region: "jp", SelectionMode: config.SelectionAuto, Enabled: true},
		{ID: "us-3001", ListenHost: "127.0.0.1", ListenPort: 3001, Region: "us", SelectionMode: config.SelectionAuto, Enabled: true},
	}
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save initial config: %v", err)
	}
	server := NewServer(manager, nodes, nil, WithConfig(path, cfg))

	req := httptest.NewRequest(http.MethodDelete, "/admin/api/channels/us-3001", nil)
	req.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("load saved config: %v", err)
	}
	if len(loaded.Channels) != 1 || loaded.Channels[0].ID != "jp-3000" {
		t.Fatalf("channels = %+v, want only jp-3000", loaded.Channels)
	}

	channelsReq := httptest.NewRequest(http.MethodGet, "/admin/api/channels", nil)
	channelsReq.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	channelsRec := httptest.NewRecorder()
	server.ServeHTTP(channelsRec, channelsReq)
	if channelsRec.Code != http.StatusOK {
		t.Fatalf("channels status = %d body=%s", channelsRec.Code, channelsRec.Body.String())
	}
	var body struct {
		Channels []channelView `json:"channels"`
	}
	if err := json.NewDecoder(channelsRec.Body).Decode(&body); err != nil {
		t.Fatalf("decode channels: %v", err)
	}
	if len(body.Channels) != 1 || body.Channels[0].ID != "jp-3000" {
		t.Fatalf("channels view = %+v, want deleted channel hidden before restart", body.Channels)
	}
}

func TestDeleteChannelReloadsRuntimeWithoutRestartingService(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	cfg.Channels = []config.Channel{
		{ID: "jp-3000", ListenHost: "127.0.0.1", ListenPort: 3000, Region: "jp", SelectionMode: config.SelectionAuto, Enabled: true},
	}
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save initial config: %v", err)
	}
	restarted := false
	reloaded := false
	server := NewServer(manager, nodes, nil, WithConfig(path, cfg), WithRestarter(func(ctx context.Context) error {
		restarted = true
		return nil
	}), WithRuntimeReloader(func(ctx context.Context) error {
		reloaded = true
		return nil
	}))

	req := httptest.NewRequest(http.MethodDelete, "/admin/api/channels/jp-3000", nil)
	req.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if restarted {
		t.Fatalf("deleting channel should not restart whole service")
	}
	if !reloaded {
		t.Fatalf("deleting channel should reload runtime")
	}
}

func TestDeleteLastChannelPersistsEmptyConfig(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	cfg.Channels = []config.Channel{
		{ID: "jp-3000", ListenHost: "127.0.0.1", ListenPort: 3000, Region: "jp", SelectionMode: config.SelectionAuto, Enabled: true},
	}
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save initial config: %v", err)
	}
	server := NewServer(manager, nodes, nil, WithConfig(path, cfg))

	req := httptest.NewRequest(http.MethodDelete, "/admin/api/channels/jp-3000", nil)
	req.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("load saved config: %v", err)
	}
	if len(loaded.Channels) != 0 {
		t.Fatalf("channels = %+v, want empty", loaded.Channels)
	}

	channelsReq := httptest.NewRequest(http.MethodGet, "/admin/api/channels", nil)
	channelsReq.SetBasicAuth(cfg.AdminUsername, cfg.AdminPassword)
	channelsRec := httptest.NewRecorder()
	server.ServeHTTP(channelsRec, channelsReq)
	if channelsRec.Code != http.StatusOK {
		t.Fatalf("channels status = %d body=%s", channelsRec.Code, channelsRec.Body.String())
	}
	var body struct {
		Channels []channelView `json:"channels"`
	}
	if err := json.NewDecoder(channelsRec.Body).Decode(&body); err != nil {
		t.Fatalf("decode channels: %v", err)
	}
	if len(body.Channels) != 0 {
		t.Fatalf("channels view = %+v, want empty before restart", body.Channels)
	}
}

func newAdminTestManager(t *testing.T) (*node.Store, *channel.Manager) {
	t.Helper()
	nodes := node.NewStore()
	nodes.Replace([]node.Node{{ID: "jp-1", Region: "jp", IP: "203.0.113.10", Hostname: "jp-demo", Available: true}})
	manager := channel.NewManager(channel.Config{
		Channels: []config.Channel{{
			ID:            "jp-3000",
			ListenHost:    "127.0.0.1",
			ListenPort:    3000,
			Region:        "jp",
			SelectionMode: config.SelectionAuto,
			Enabled:       true,
		}},
		Nodes: nodes,
		TunnelFactory: func(name string) tunnel.Tunnel {
			return tunnel.NewFake(name)
		},
		DataDir: t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("manager start: %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background()) })
	return nodes, manager
}

func openAdminTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "region-proxy-gateway.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
