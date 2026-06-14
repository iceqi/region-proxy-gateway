package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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
	defer services.proxyRuntime.Stop(context.Background())

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

func TestBuildServicesSharesTrackerBetweenAdminAndProxy(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	services, err := buildServices(context.Background(), cfg, "")
	if err != nil {
		t.Fatalf("buildServices returned error: %v", err)
	}
	defer services.channels.Stop(context.Background())
	defer services.proxyRuntime.Stop(context.Background())

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
