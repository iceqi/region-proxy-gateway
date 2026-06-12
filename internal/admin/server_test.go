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

	"github.com/iceqi/region-proxy-gateway/internal/channel"
	"github.com/iceqi/region-proxy-gateway/internal/config"
	"github.com/iceqi/region-proxy-gateway/internal/connection"
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

func TestRefreshNodesReplacesNodeStore(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	server := NewServer(manager, nodes, nil, WithNodeRefresher(func(ctx context.Context) ([]node.Node, error) {
		return []node.Node{{ID: "us-new", Region: "us", Available: true}}, nil
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
	for _, text := range []string{"content-panel", "text-overflow: ellipsis", "title: value"} {
		if !strings.Contains(rec.Body.String(), text) {
			t.Fatalf("admin html missing layout safeguard %q", text)
		}
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

	req := httptest.NewRequest(http.MethodGet, "/admin/api/nodes", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status without auth = %d, want 401", rec.Code)
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
	nodes.Replace(append(nodes.List(), node.Node{ID: "jp-2", Region: "jp", Available: true}))
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
	nodes.Replace([]node.Node{{ID: "jp-1", Region: "jp", Available: true}})
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
