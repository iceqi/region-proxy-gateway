package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
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

	req := httptest.NewRequest(http.MethodGet, "/api/channels", nil)
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

	req := httptest.NewRequest(http.MethodGet, "/api/nodes?region=jp", nil)
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

func TestIndexReturnsHTML(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	server := NewServer(manager, nodes, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q, want html", got)
	}
}

func TestSwitchChannelToNode(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	nodes.Replace(append(nodes.List(), node.Node{ID: "jp-2", Region: "jp", Available: true}))
	server := NewServer(manager, nodes, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/channels/jp-3000/switch", bytes.NewBufferString(`{"node_id":"jp-2"}`))
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
