package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iceqi/region-proxy-gateway/internal/connection"
	"github.com/iceqi/region-proxy-gateway/internal/node"
	"github.com/iceqi/region-proxy-gateway/internal/session"
)

func TestStatusReturnsActiveSessions(t *testing.T) {
	nodes := node.NewStore()
	sessions := session.NewManager(nodes, 10, nil)
	connections := connection.NewTracker()
	connections.Start("127.0.0.1:12345", "http", "jp:10", "example.com:443")
	server := NewServer(sessions, nodes, connections)

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("expected json content type, got %q", got)
	}

	var body struct {
		OK              bool `json:"ok"`
		ActiveSessions  int  `json:"active_sessions"`
		NodeCount       int  `json:"node_count"`
		ConnectionCount int  `json:"connection_count"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.OK {
		t.Fatal("expected ok=true")
	}
	if body.ActiveSessions != 0 {
		t.Fatalf("expected active_sessions=0, got %d", body.ActiveSessions)
	}
	if body.ConnectionCount != 1 {
		t.Fatalf("expected connection_count=1, got %d", body.ConnectionCount)
	}
}

func TestSessionsReturnsOK(t *testing.T) {
	nodes := node.NewStore()
	sessions := session.NewManager(nodes, 10, nil)
	server := NewServer(sessions, nodes, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body struct {
		Sessions []session.Session `json:"sessions"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Sessions == nil {
		t.Fatal("expected sessions array")
	}
}

func TestConnectionsReturnsActiveConnections(t *testing.T) {
	nodes := node.NewStore()
	sessions := session.NewManager(nodes, 10, nil)
	connections := connection.NewTracker()
	connections.Start("127.0.0.1:12345", "http", "jp:10", "example.com:443")
	server := NewServer(sessions, nodes, connections)

	req := httptest.NewRequest(http.MethodGet, "/api/connections", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body struct {
		Connections []connection.Record `json:"connections"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Connections) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(body.Connections))
	}
	if body.Connections[0].ClientAddr != "127.0.0.1:12345" {
		t.Fatalf("expected client addr to round-trip, got %q", body.Connections[0].ClientAddr)
	}
}

func TestConnectionsReturnsEmptyListWithoutTracker(t *testing.T) {
	nodes := node.NewStore()
	sessions := session.NewManager(nodes, 10, nil)
	server := NewServer(sessions, nodes, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/connections", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body struct {
		Connections []connection.Record `json:"connections"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Connections == nil {
		t.Fatal("expected connections array")
	}
	if len(body.Connections) != 0 {
		t.Fatalf("expected 0 connections, got %d", len(body.Connections))
	}
}

func TestNonGETReturnsMethodNotAllowed(t *testing.T) {
	nodes := node.NewStore()
	sessions := session.NewManager(nodes, 10, nil)
	server := NewServer(sessions, nodes, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/status", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", rec.Code)
	}

	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error == "" {
		t.Fatal("expected error message")
	}
}
