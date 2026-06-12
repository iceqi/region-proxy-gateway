package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iceqi/region-proxy-gateway/internal/config"
)

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
	if len(services.proxies) != 1 || services.proxies[0].ListenAddr != wantProxyAddr {
		t.Fatalf("proxies = %+v, want one proxy at %s", services.proxies, wantProxyAddr)
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
