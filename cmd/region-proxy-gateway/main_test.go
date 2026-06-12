package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iceqi/region-proxy-gateway/internal/config"
)

func TestBuildServicesReportsDemoNodesAndProxyAddr(t *testing.T) {
	cfg := config.Default()
	cfg.ProxyHost = "127.0.0.1"
	cfg.ProxyPort = 12345
	services := buildServices(cfg)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	services.admin.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusOK)
	}

	var body struct {
		OK              bool `json:"ok"`
		ActiveSessions  int  `json:"active_sessions"`
		NodeCount       int  `json:"node_count"`
		ConnectionCount int  `json:"connection_count"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !body.OK {
		t.Fatalf("ok = false, want true")
	}
	if body.ActiveSessions != 0 {
		t.Fatalf("active_sessions = %d, want 0", body.ActiveSessions)
	}
	if body.NodeCount != 2 {
		t.Fatalf("node_count = %d, want 2", body.NodeCount)
	}
	if body.ConnectionCount != 0 {
		t.Fatalf("connection_count = %d, want 0", body.ConnectionCount)
	}
	wantProxyAddr := fmt.Sprintf("%s:%d", cfg.ProxyHost, cfg.ProxyPort)
	if services.proxy.ListenAddr != wantProxyAddr {
		t.Fatalf("proxy ListenAddr = %q, want %q", services.proxy.ListenAddr, wantProxyAddr)
	}
}

func TestBuildServicesSharesTrackerBetweenAdminAndProxy(t *testing.T) {
	services := buildServices(config.Default())

	services.tracker.Start("127.0.0.1:50000", "http", "jp:10", "example.com:443")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/status", nil)
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
