package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/iceqi/region-proxy-gateway/internal/config"
	"github.com/iceqi/region-proxy-gateway/internal/node"
)

func TestSQLiteStoreMigratesChannelsOnce(t *testing.T) {
	store := openTestStore(t)
	initial := []config.Channel{
		{ID: "jp-3000", ListenHost: "0.0.0.0", ListenPort: 3000, Region: "jp", RotateMinutes: 10, SelectionMode: config.SelectionAuto, Enabled: true},
	}

	if err := store.MigrateChannels(context.Background(), initial); err != nil {
		t.Fatalf("MigrateChannels: %v", err)
	}
	if err := store.MigrateChannels(context.Background(), []config.Channel{
		{ID: "us-3001", ListenHost: "0.0.0.0", ListenPort: 3001, Region: "us", SelectionMode: config.SelectionAuto, Enabled: true},
	}); err != nil {
		t.Fatalf("second MigrateChannels: %v", err)
	}

	channels, err := store.ListChannels(context.Background())
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(channels) != 1 || channels[0].ID != "jp-3000" {
		t.Fatalf("channels = %+v, want only migrated jp-3000", channels)
	}
}

func TestSQLiteStoreUpsertRenameAndDeleteChannels(t *testing.T) {
	store := openTestStore(t)
	first := config.Channel{ID: "jp-3000", ListenHost: "0.0.0.0", ListenPort: 3000, Region: "jp", RotateMinutes: 10, SelectionMode: config.SelectionAuto, Enabled: true}
	if err := store.SaveChannel(context.Background(), "", first); err != nil {
		t.Fatalf("SaveChannel first: %v", err)
	}
	renamed := first
	renamed.ID = "jp-main"
	renamed.RotateMinutes = 5
	if err := store.SaveChannel(context.Background(), "jp-3000", renamed); err != nil {
		t.Fatalf("SaveChannel rename: %v", err)
	}

	channels, err := store.ListChannels(context.Background())
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(channels) != 1 || channels[0].ID != "jp-main" || channels[0].RotateMinutes != 5 {
		t.Fatalf("channels = %+v, want renamed jp-main", channels)
	}

	if err := store.DeleteChannel(context.Background(), "jp-main"); err != nil {
		t.Fatalf("DeleteChannel: %v", err)
	}
	channels, err = store.ListChannels(context.Background())
	if err != nil {
		t.Fatalf("ListChannels after delete: %v", err)
	}
	if len(channels) != 0 {
		t.Fatalf("channels = %+v, want empty", channels)
	}
}

func TestSQLiteStoreReplacesAndListsNodes(t *testing.T) {
	store := openTestStore(t)
	nodes := []node.Node{
		{ID: "jp-1", Region: "jp", Country: "Japan", IP: "203.0.113.1", Hostname: "vpn-jp", Port: 1194, Proto: "udp", OpenVPN: "client\n", LatencyMS: 20, Speed: 1000, Available: true, Owner: "Home ISP", ASN: "AS1", ASName: "HOME", Location: "日本", IPType: "residential", Quality: "normal", PurityScore: 90, ProbeStatus: "available", ProbeMessage: "Ping 正常"},
		{ID: "us-1", Region: "us", Country: "United States", IP: "198.51.100.1", Hostname: "vpn-us", Port: 443, Proto: "tcp", OpenVPN: "client\n", LatencyMS: 50, Speed: 500, Available: true, Owner: "Cloud", ASN: "AS2", ASName: "CLOUD", Location: "美国", IPType: "hosting", Quality: "datacenter", PurityScore: 45},
	}
	if err := store.ReplaceNodes(context.Background(), nodes); err != nil {
		t.Fatalf("ReplaceNodes: %v", err)
	}

	got, err := store.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(got) != 2 || got[0].ID != "jp-1" || got[1].ID != "us-1" {
		t.Fatalf("nodes = %+v, want jp-1 us-1", got)
	}
	if got[0].OpenVPN != "client\n" || got[0].PurityScore != 90 {
		t.Fatalf("node fields not round-tripped: %+v", got[0])
	}
	if got[0].ProbeStatus != "" || got[0].ProbeMessage != "" {
		t.Fatalf("probe fields should stay realtime and not be cached: %+v", got[0])
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "region-proxy-gateway.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
