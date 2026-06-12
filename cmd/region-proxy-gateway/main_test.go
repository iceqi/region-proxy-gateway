package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iceqi/region-proxy-gateway/internal/config"
)

func TestBuildAdminServerReportsDemoNodes(t *testing.T) {
	server := buildAdminServer(config.Default())

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusOK)
	}

	var body struct {
		OK             bool `json:"ok"`
		ActiveSessions int  `json:"active_sessions"`
		NodeCount      int  `json:"node_count"`
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
}
