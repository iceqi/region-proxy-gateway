package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/config"
	"github.com/iceqi/region-proxy-gateway/internal/deeptest"
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

func TestSQLiteStoreSaveChannelNormalizesRegion(t *testing.T) {
	store := openTestStore(t)
	ch := config.Channel{ID: "jp-3000", ListenHost: "0.0.0.0", ListenPort: 3000, Region: " JP ", SelectionMode: config.SelectionAuto, Enabled: true}
	if err := store.SaveChannel(context.Background(), "", ch); err != nil {
		t.Fatalf("SaveChannel: %v", err)
	}

	channels, err := store.ListChannels(context.Background())
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(channels) != 1 || channels[0].Region != "jp" {
		t.Fatalf("channels = %+v, want normalized region jp", channels)
	}
}

func TestSQLiteStoreSaveChannelAllowsWildcardRegion(t *testing.T) {
	store := openTestStore(t)
	for _, region := range []string{"", " * "} {
		ch := config.Channel{ID: "any-3000" + region, ListenHost: "0.0.0.0", ListenPort: 3000 + len(region), Region: region, SelectionMode: config.SelectionAuto, Enabled: true}
		if err := store.SaveChannel(context.Background(), "", ch); err != nil {
			t.Fatalf("SaveChannel region %q: %v", region, err)
		}
	}
	channels, err := store.ListChannels(context.Background())
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	seen := map[string]bool{}
	for _, ch := range channels {
		seen[ch.Region] = true
	}
	if !seen[""] || !seen["*"] {
		t.Fatalf("channels = %+v, want empty and * regions", channels)
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
	if got[0].LatencyMS != 0 {
		t.Fatalf("cached latency = %d, want 0 because latency is realtime only", got[0].LatencyMS)
	}
	if got[0].ProbeStatus != "" || got[0].ProbeMessage != "" {
		t.Fatalf("probe fields should stay realtime and not be cached: %+v", got[0])
	}
}

func TestSQLiteStoreDeepTestQueueDeduplicatesPendingJobs(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	first, err := store.EnqueueDeepTestJobs(ctx, []string{"jp-1", "jp-1", "jp-2"})
	if err != nil {
		t.Fatalf("EnqueueDeepTestJobs first: %v", err)
	}
	second, err := store.EnqueueDeepTestJobs(ctx, []string{"jp-1", "jp-3"})
	if err != nil {
		t.Fatalf("EnqueueDeepTestJobs second: %v", err)
	}

	if first.Created != 2 || first.Skipped != 1 {
		t.Fatalf("first summary = %+v, want 2 created 1 skipped", first)
	}
	if second.Created != 1 || second.Skipped != 1 {
		t.Fatalf("second summary = %+v, want 1 created 1 skipped", second)
	}

	jobs, err := store.ClaimDeepTestJobs(ctx, 10, time.Now())
	if err != nil {
		t.Fatalf("ClaimDeepTestJobs: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("jobs = %d, want 3", len(jobs))
	}
}

func TestSQLiteStoreSavesDeepTestResult(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	if _, err := store.EnqueueDeepTestJobs(ctx, []string{"jp-1"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	jobs, err := store.ClaimDeepTestJobs(ctx, 1, time.Now())
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(jobs))
	}

	result := deeptest.Result{
		NodeID:      "jp-1",
		Status:      deeptest.StatusSuccess,
		ExitIP:      "203.0.113.99",
		ExitCountry: "Japan",
		ConnectMS:   1234,
		TestedAt:    time.Date(2026, 6, 13, 1, 2, 3, 0, time.UTC),
	}
	if err := store.CompleteDeepTestJob(ctx, jobs[0].ID, result); err != nil {
		t.Fatalf("CompleteDeepTestJob: %v", err)
	}

	got, ok, err := store.DeepTestResult(ctx, "jp-1")
	if err != nil {
		t.Fatalf("DeepTestResult: %v", err)
	}
	if !ok || got.Status != deeptest.StatusSuccess || got.ExitIP != "203.0.113.99" || got.ConnectMS != 1234 {
		t.Fatalf("result = %+v ok=%v, want saved success", got, ok)
	}
}

func TestSQLiteStoreRecordsChannelHistoryAndRecentUse(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)

	if err := store.RecordChannelNodeUse(ctx, ChannelNodeUse{
		ChannelID:   "jp-3000",
		NodeID:      "jp-1",
		ExitIP:      "203.0.113.10",
		ConnectedAt: now.Add(-2 * time.Hour),
		SwitchedAt:  now.Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("RecordChannelNodeUse recent: %v", err)
	}
	if err := store.RecordChannelNodeUse(ctx, ChannelNodeUse{
		ChannelID:   "jp-3000",
		NodeID:      "jp-old",
		ExitIP:      "203.0.113.11",
		ConnectedAt: now.Add(-30 * time.Hour),
		SwitchedAt:  now.Add(-30 * time.Hour),
	}); err != nil {
		t.Fatalf("RecordChannelNodeUse old: %v", err)
	}

	recent, err := store.RecentNodeIDsForChannel(ctx, "jp-3000", now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("RecentNodeIDsForChannel: %v", err)
	}
	if len(recent) != 1 || recent["jp-1"].IsZero() {
		t.Fatalf("recent = %+v, want only jp-1", recent)
	}

	current, ok, err := store.CurrentChannelNodeUse(ctx, "jp-3000")
	if err != nil {
		t.Fatalf("CurrentChannelNodeUse: %v", err)
	}
	if !ok || current.NodeID != "jp-1" || current.ExitIP != "203.0.113.10" {
		t.Fatalf("current = %+v ok=%v, want jp-1", current, ok)
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
