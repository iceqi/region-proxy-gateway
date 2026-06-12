package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/iceqi/region-proxy-gateway/internal/channel"
	"github.com/iceqi/region-proxy-gateway/internal/config"
	"github.com/iceqi/region-proxy-gateway/internal/connection"
	"github.com/iceqi/region-proxy-gateway/internal/node"
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
	server := NewServer(manager, nodes, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/channels", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var body struct {
		Channels []channel.Snapshot `json:"channels"`
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

func TestIndexReturnsHTML(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	server := NewServer(manager, nodes, nil, WithAdminPath("/secret-admin"))

	req := httptest.NewRequest(http.MethodGet, "/secret-admin", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q, want html", got)
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
