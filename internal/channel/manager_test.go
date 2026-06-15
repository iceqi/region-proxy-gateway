package channel

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/config"
	"github.com/iceqi/region-proxy-gateway/internal/deeptest"
	"github.com/iceqi/region-proxy-gateway/internal/node"
	"github.com/iceqi/region-proxy-gateway/internal/tunnel"
)

func TestManagerStartsAutoChannelWithBestRegionalNode(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-slow", Region: "jp", Hostname: "slow", Speed: 100, LatencyMS: 0, Available: true},
		{ID: "jp-fast", Region: "jp", Hostname: "fast", Speed: 1000, LatencyMS: 0, Available: true},
		{ID: "us-fast", Region: "us", Hostname: "us", Speed: 5000, LatencyMS: 10, Available: true},
	})
	factory := &recordingFactory{}
	manager := NewManager(Config{
		Channels: []config.Channel{{
			ID:            "jp-3000",
			ListenHost:    "127.0.0.1",
			ListenPort:    3000,
			Region:        "jp",
			RotateMinutes: 10,
			SelectionMode: SelectionAuto,
			Enabled:       true,
		}},
		Nodes:         nodes,
		TunnelFactory: factory.New,
		NodeChecker: func(ctx context.Context, n node.Node) node.Node {
			if n.ID == "jp-slow" {
				n.LatencyMS = 15
			} else {
				n.LatencyMS = 80
			}
			n.Available = true
			n.ProbeStatus = "available"
			return n
		},
		DataDir: t.TempDir(),
	})

	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	snapshots := manager.Snapshots()
	if len(snapshots) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(snapshots))
	}
	if snapshots[0].CurrentNodeID != "jp-slow" {
		t.Fatalf("current node = %q, want jp-slow after realtime check", snapshots[0].CurrentNodeID)
	}
	if factory.tunnels[0].startedNode.ID != "jp-slow" {
		t.Fatalf("started node = %q, want jp-slow", factory.tunnels[0].startedNode.ID)
	}
}

func TestManagerAutoSelectionAvoidsUnknownUDPWhenAvailableNodeExists(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-unknown", Region: "jp", Speed: 5000, Available: true},
		{ID: "jp-available", Region: "jp", Speed: 100, Available: true},
	})
	factory := &recordingFactory{}
	manager := NewManager(Config{
		Channels: []config.Channel{{
			ID:            "jp-3000",
			ListenHost:    "127.0.0.1",
			ListenPort:    3000,
			Region:        "jp",
			RotateMinutes: 10,
			SelectionMode: SelectionAuto,
			Enabled:       true,
		}},
		Nodes:         nodes,
		TunnelFactory: factory.New,
		NodeChecker: func(ctx context.Context, n node.Node) node.Node {
			if n.ID == "jp-unknown" {
				n.Available = false
				n.ProbeStatus = "unknown"
				return n
			}
			n.Available = true
			n.ProbeStatus = "available"
			n.LatencyMS = 100
			return n
		},
		DataDir: t.TempDir(),
	})

	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	snapshot, _ := manager.Snapshot("jp-3000")
	if snapshot.CurrentNodeID != "jp-available" {
		t.Fatalf("current node = %q, want available node", snapshot.CurrentNodeID)
	}
}

func TestManagerSwitchesChannelToManualNode(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-a", Region: "jp", Available: true},
		{ID: "jp-b", Region: "jp", Available: true},
	})
	factory := &recordingFactory{}
	manager := NewManager(Config{
		Channels: []config.Channel{{
			ID:            "jp-3000",
			ListenHost:    "127.0.0.1",
			ListenPort:    3000,
			Region:        "jp",
			RotateMinutes: 10,
			SelectionMode: SelectionAuto,
			Enabled:       true,
		}},
		Nodes:         nodes,
		TunnelFactory: factory.New,
		DataDir:       t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	if err := manager.SwitchToNode(context.Background(), "jp-3000", "jp-b"); err != nil {
		t.Fatalf("SwitchToNode: %v", err)
	}

	snapshot, ok := manager.Snapshot("jp-3000")
	if !ok {
		t.Fatalf("missing channel snapshot")
	}
	if snapshot.CurrentNodeID != "jp-b" {
		t.Fatalf("current node = %q, want jp-b", snapshot.CurrentNodeID)
	}
	if factory.tunnels[0].switchedNode.ID != "jp-b" {
		t.Fatalf("switched node = %q, want jp-b", factory.tunnels[0].switchedNode.ID)
	}
}

func TestManagerSwitchToNodeStartsStoppedChannel(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-a", Region: "jp", Available: true},
		{ID: "jp-b", Region: "jp", Available: true},
	})
	factory := &recordingFactory{}
	manager := NewManager(Config{
		Channels: []config.Channel{{
			ID:            "jp-3000",
			ListenHost:    "127.0.0.1",
			ListenPort:    3000,
			Region:        "jp",
			SelectionMode: SelectionManual,
			ManualNodeID:  "jp-a",
			Enabled:       false,
		}},
		Nodes:         nodes,
		TunnelFactory: factory.New,
		DataDir:       t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	if err := manager.SwitchToNode(context.Background(), "jp-3000", "jp-b"); err != nil {
		t.Fatalf("SwitchToNode: %v", err)
	}
	snapshot, ok := manager.Snapshot("jp-3000")
	if !ok {
		t.Fatalf("missing channel snapshot")
	}
	if snapshot.CurrentNodeID != "jp-b" {
		t.Fatalf("current node = %q, want jp-b", snapshot.CurrentNodeID)
	}
	if len(factory.tunnels) != 1 || factory.tunnels[0].startedNode.ID != "jp-b" {
		t.Fatalf("tunnel start = %+v, want started jp-b", factory.tunnels)
	}
}

func TestManagerReplaceChannelsAddsUpdatesAndRemovesWithoutProcessRestart(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-a", Region: "jp", Available: true},
		{ID: "us-a", Region: "us", Available: true},
	})
	factory := &recordingFactory{}
	manager := NewManager(Config{
		Channels: []config.Channel{{
			ID:            "jp-3000",
			ListenHost:    "127.0.0.1",
			ListenPort:    3000,
			Region:        "jp",
			RotateMinutes: 10,
			SelectionMode: SelectionAuto,
			Enabled:       true,
		}},
		Nodes:         nodes,
		TunnelFactory: factory.New,
		DataDir:       t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	if err := manager.ReplaceChannels(context.Background(), []config.Channel{{
		ID:            "us-3001",
		ListenHost:    "127.0.0.1",
		ListenPort:    3001,
		Region:        "us",
		SelectionMode: SelectionAuto,
		Enabled:       true,
	}}); err != nil {
		t.Fatalf("ReplaceChannels: %v", err)
	}

	if _, ok := manager.Snapshot("jp-3000"); ok {
		t.Fatalf("removed channel should not have snapshot")
	}
	snapshot, ok := manager.Snapshot("us-3001")
	if !ok {
		t.Fatalf("added channel should have snapshot")
	}
	if snapshot.CurrentNodeID != "us-a" {
		t.Fatalf("added channel node = %q, want us-a", snapshot.CurrentNodeID)
	}
	if !factory.tunnels[0].stopped {
		t.Fatalf("removed channel tunnel should be stopped")
	}
	if len(factory.tunnels) != 2 || factory.tunnels[1].startedNode.ID != "us-a" {
		t.Fatalf("factory tunnels = %+v, want second tunnel started with us-a", factory.tunnels)
	}
}

func TestManagerRotatesAutoChannelToBestAlternativeNode(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-a", Region: "jp", Speed: 100, LatencyMS: 20, Available: true},
		{ID: "jp-b", Region: "jp", Speed: 200, LatencyMS: 10, Available: true},
	})
	factory := &recordingFactory{}
	manager := NewManager(Config{
		Channels: []config.Channel{{
			ID:            "jp-3000",
			ListenHost:    "127.0.0.1",
			ListenPort:    3000,
			Region:        "jp",
			RotateMinutes: 10,
			SelectionMode: SelectionAuto,
			Enabled:       true,
		}},
		Nodes:         nodes,
		TunnelFactory: factory.New,
		DataDir:       t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	if err := manager.RotateNow(context.Background(), "jp-3000"); err != nil {
		t.Fatalf("RotateNow: %v", err)
	}

	snapshot, ok := manager.Snapshot("jp-3000")
	if !ok {
		t.Fatalf("missing channel snapshot")
	}
	if snapshot.CurrentNodeID != "jp-a" {
		t.Fatalf("current node = %q, want jp-a", snapshot.CurrentNodeID)
	}
	if factory.tunnels[0].switchedNode.ID != "jp-a" {
		t.Fatalf("switched node = %q, want jp-a", factory.tunnels[0].switchedNode.ID)
	}
}

func TestManagerWildcardRegionCanStartAndRotateAcrossRegions(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-a", Region: "jp", Speed: 100, LatencyMS: 20, Available: true},
		{ID: "us-a", Region: "us", Speed: 300, LatencyMS: 10, Available: true},
	})
	factory := &recordingFactory{}
	manager := NewManager(Config{
		Channels: []config.Channel{{
			ID:            "any-3000",
			ListenHost:    "127.0.0.1",
			ListenPort:    3000,
			Region:        "*",
			RotateMinutes: 10,
			SelectionMode: SelectionAuto,
			Enabled:       true,
		}},
		Nodes:         nodes,
		TunnelFactory: factory.New,
		DataDir:       t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	initial, _ := manager.Snapshot("any-3000")
	if initial.CurrentNodeID != "us-a" {
		t.Fatalf("current node = %q, want best any-region us-a", initial.CurrentNodeID)
	}
	if err := manager.RotateNow(context.Background(), "any-3000"); err != nil {
		t.Fatalf("RotateNow: %v", err)
	}
	snapshot, _ := manager.Snapshot("any-3000")
	if snapshot.CurrentNodeID != "jp-a" {
		t.Fatalf("current node = %q, want rotated cross-region jp-a", snapshot.CurrentNodeID)
	}
}

func TestManagerManualWildcardRegionAcceptsAnyNodeRegion(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{{ID: "us-a", Region: "us", Speed: 300, Available: true}})
	factory := &recordingFactory{}
	manager := NewManager(Config{
		Channels: []config.Channel{{
			ID:            "any-3000",
			ListenHost:    "127.0.0.1",
			ListenPort:    3000,
			Region:        "*",
			SelectionMode: SelectionManual,
			ManualNodeID:  "us-a",
			Enabled:       true,
		}},
		Nodes:         nodes,
		TunnelFactory: factory.New,
		DataDir:       t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	snapshot, _ := manager.Snapshot("any-3000")
	if snapshot.CurrentNodeID != "us-a" {
		t.Fatalf("current node = %q, want manual us-a", snapshot.CurrentNodeID)
	}
}

func TestManagerSwitchToNodeAllowsEmptyWildcardRegion(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-a", Region: "jp", Available: true},
		{ID: "kr-a", Region: "kr", Available: true},
	})
	factory := &recordingFactory{}
	manager := NewManager(Config{
		Channels: []config.Channel{{
			ID:            "any-3000",
			ListenHost:    "127.0.0.1",
			ListenPort:    3000,
			Region:        "",
			SelectionMode: SelectionAuto,
			Enabled:       true,
		}},
		Nodes:         nodes,
		TunnelFactory: factory.New,
		DataDir:       t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	if err := manager.SwitchToNode(context.Background(), "any-3000", "kr-a"); err != nil {
		t.Fatalf("SwitchToNode should allow empty wildcard region: %v", err)
	}
	snapshot, _ := manager.Snapshot("any-3000")
	if snapshot.CurrentNodeID != "kr-a" {
		t.Fatalf("current node = %q, want kr-a", snapshot.CurrentNodeID)
	}
}

func TestManagerKeepsRunningWhenOneChannelCannotStart(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{{ID: "jp-a", Region: "jp", Available: true}})
	factory := &recordingFactory{}
	manager := NewManager(Config{
		Channels: []config.Channel{
			{ID: "jp-3000", ListenHost: "127.0.0.1", ListenPort: 3000, Region: "jp", SelectionMode: SelectionAuto, Enabled: true},
			{ID: "us-3001", ListenHost: "127.0.0.1", ListenPort: 3001, Region: "us", SelectionMode: SelectionAuto, Enabled: true},
		},
		Nodes:         nodes,
		TunnelFactory: factory.New,
		DataDir:       t.TempDir(),
	})

	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start should keep service available when one channel fails: %v", err)
	}
	defer manager.Stop(context.Background())

	working, _ := manager.Snapshot("jp-3000")
	failed, ok := manager.Snapshot("us-3001")
	if !ok {
		t.Fatalf("missing failed channel snapshot")
	}
	if working.CurrentNodeID != "jp-a" {
		t.Fatalf("working channel node = %q, want jp-a", working.CurrentNodeID)
	}
	if failed.LastError == "" {
		t.Fatalf("failed channel should expose last error")
	}
}

func TestManagerRotationFailureKeepsCurrentNode(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-a", Region: "jp", Speed: 100, Available: true},
		{ID: "jp-b", Region: "jp", Speed: 200, Available: true},
	})
	factory := &recordingFactory{switchErr: fmt.Errorf("switch failed")}
	manager := NewManager(Config{
		Channels: []config.Channel{{
			ID:            "jp-3000",
			ListenHost:    "127.0.0.1",
			ListenPort:    3000,
			Region:        "jp",
			RotateMinutes: 10,
			SelectionMode: SelectionAuto,
			Enabled:       true,
		}},
		Nodes:         nodes,
		TunnelFactory: factory.New,
		DataDir:       t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	err := manager.RotateNow(context.Background(), "jp-3000")
	if err == nil {
		t.Fatalf("RotateNow should return switch error")
	}

	snapshot, _ := manager.Snapshot("jp-3000")
	if snapshot.CurrentNodeID != "jp-b" {
		t.Fatalf("current node = %q, want jp-b after failed rotation", snapshot.CurrentNodeID)
	}
	if snapshot.LastError == "" {
		t.Fatalf("last error should be recorded")
	}
}

func TestManagerRotationRequiresDifferentNode(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{{ID: "jp-only", Region: "jp", Speed: 100, Available: true}})
	factory := &recordingFactory{}
	manager := NewManager(Config{
		Channels: []config.Channel{{
			ID:            "jp-3000",
			ListenHost:    "127.0.0.1",
			ListenPort:    3000,
			Region:        "jp",
			RotateMinutes: 10,
			SelectionMode: SelectionAuto,
			Enabled:       true,
		}},
		Nodes:         nodes,
		TunnelFactory: factory.New,
		DataDir:       t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	initial, _ := manager.Snapshot("jp-3000")
	err := manager.RotateNow(context.Background(), "jp-3000")
	if err == nil || !strings.Contains(err.Error(), "no alternative node") {
		t.Fatalf("RotateNow error = %v, want no alternative node", err)
	}

	snapshot, _ := manager.Snapshot("jp-3000")
	if snapshot.CurrentNodeID != "jp-only" {
		t.Fatalf("current node = %q, want unchanged", snapshot.CurrentNodeID)
	}
	if snapshot.LastError == "" {
		t.Fatalf("last error should be recorded")
	}
	if !snapshot.LastRotationAt.After(initial.LastRotationAt) {
		t.Fatalf("last rotation attempt time should advance")
	}
	if !snapshot.NextRotationAt.Equal(snapshot.LastRotationAt.Add(10 * time.Minute)) {
		t.Fatalf("next rotation = %v, want last attempt + interval", snapshot.NextRotationAt)
	}
}

func TestManagerRetriesDialFailuresThenRotatesAutoChannel(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-a", Region: "jp", Speed: 100, Available: true},
		{ID: "jp-b", Region: "jp", Speed: 200, Available: true},
	})
	factory := &recordingFactory{dialErrs: 4}
	manager := NewManager(Config{
		Channels: []config.Channel{{
			ID:            "jp-3000",
			ListenHost:    "127.0.0.1",
			ListenPort:    3000,
			Region:        "jp",
			RotateMinutes: 10,
			SelectionMode: SelectionAuto,
			Enabled:       true,
		}},
		Nodes:         nodes,
		TunnelFactory: factory.New,
		DataDir:       t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	conn, err := manager.DialContext(context.Background(), "jp-3000", "tcp", "example.com:443")
	if err != nil {
		t.Fatalf("DialContext: %v", err)
	}
	_ = conn.Close()

	snapshot, _ := manager.Snapshot("jp-3000")
	if snapshot.CurrentNodeID != "jp-a" {
		t.Fatalf("current node = %q, want jp-a after failed retries rotate away from jp-b", snapshot.CurrentNodeID)
	}
	if snapshot.LastError != "" {
		t.Fatalf("last error = %q, want empty after successful retry", snapshot.LastError)
	}
	if factory.tunnels[0].dialCount != 5 {
		t.Fatalf("dial count = %d, want 5", factory.tunnels[0].dialCount)
	}
	if factory.tunnels[0].switchedNode.ID != "jp-a" {
		t.Fatalf("switched node = %q, want jp-a", factory.tunnels[0].switchedNode.ID)
	}
}

func TestManagerAutoChannelTriesNextNodeWhenRecoveredNodeStillFails(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-a", Region: "jp", Speed: 300, Available: true},
		{ID: "jp-b", Region: "jp", Speed: 200, Available: true},
		{ID: "jp-c", Region: "jp", Speed: 100, Available: true},
	})
	factory := &recordingFactory{dialErrs: 4, dialErrByNode: map[string]error{"jp-b": fmt.Errorf("node b failed")}}
	manager := NewManager(Config{
		Channels: []config.Channel{{
			ID:            "jp-3000",
			ListenHost:    "127.0.0.1",
			ListenPort:    3000,
			Region:        "jp",
			RotateMinutes: 10,
			SelectionMode: SelectionAuto,
			Enabled:       true,
		}},
		Nodes:         nodes,
		TunnelFactory: factory.New,
		DataDir:       t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	conn, err := manager.DialContext(context.Background(), "jp-3000", "tcp", "example.com:443")
	if err != nil {
		t.Fatalf("DialContext: %v", err)
	}
	_ = conn.Close()

	if len(factory.tunnels) != 1 {
		t.Fatalf("tunnel count = %d, want one tunnel switching candidates", len(factory.tunnels))
	}
	wantSwitches := []string{"jp-b", "jp-c"}
	if !reflect.DeepEqual(factory.tunnels[0].switchedNodes, wantSwitches) {
		t.Fatalf("switched nodes = %#v, want %#v", factory.tunnels[0].switchedNodes, wantSwitches)
	}
	snapshot, _ := manager.Snapshot("jp-3000")
	if snapshot.CurrentNodeID != "jp-c" {
		t.Fatalf("current node = %q, want jp-c after fallbacks", snapshot.CurrentNodeID)
	}
	if snapshot.LastError != "" {
		t.Fatalf("last error = %q, want empty after successful fallback", snapshot.LastError)
	}
}

func TestManagerRestartsManualChannelAfterDialFailures(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{{ID: "jp-a", Region: "jp", Speed: 100, Available: true}})
	factory := &recordingFactory{dialErrs: 4}
	manager := NewManager(Config{
		Channels: []config.Channel{{
			ID:            "jp-3000",
			ListenHost:    "127.0.0.1",
			ListenPort:    3000,
			Region:        "jp",
			SelectionMode: SelectionManual,
			ManualNodeID:  "jp-a",
			Enabled:       true,
		}},
		Nodes:         nodes,
		TunnelFactory: factory.New,
		DataDir:       t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	conn, err := manager.DialContext(context.Background(), "jp-3000", "tcp", "example.com:443")
	if err != nil {
		t.Fatalf("DialContext: %v", err)
	}
	_ = conn.Close()

	if len(factory.tunnels) != 1 {
		t.Fatalf("tunnel count = %d, want restart on the same tunnel", len(factory.tunnels))
	}
	if factory.tunnels[0].switchedNode.ID != "jp-a" {
		t.Fatalf("restarted node = %q, want jp-a", factory.tunnels[0].switchedNode.ID)
	}
	if factory.tunnels[0].dialCount != 5 {
		t.Fatalf("dial count = %d, want 5 attempts across restart", factory.tunnels[0].dialCount)
	}
	snapshot, _ := manager.Snapshot("jp-3000")
	if snapshot.LastError != "" {
		t.Fatalf("last error = %q, want empty after successful restart", snapshot.LastError)
	}
}

func TestManagerAutoRecoveryFailureDoesNotAffectOtherChannels(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-a", Region: "jp", IP: "198.51.100.10", Speed: 300, Available: true},
		{ID: "jp-b", Region: "jp", IP: "198.51.100.20", Speed: 200, Available: true},
		{ID: "jp-c", Region: "jp", IP: "198.51.100.30", Speed: 100, Available: true},
	})
	factory := &recordingFactory{dialErrByChannel: map[string]error{"rotating": fmt.Errorf("rotating failed")}}
	manager := NewManager(Config{
		Channels: []config.Channel{
			{ID: "rotating", ListenHost: "127.0.0.1", ListenPort: 3000, Region: "jp", RotateMinutes: 10, SelectionMode: SelectionAuto, Enabled: true},
			{ID: "fixed", ListenHost: "127.0.0.1", ListenPort: 3001, Region: "jp", RotateMinutes: 0, SelectionMode: SelectionAuto, Enabled: true},
		},
		Nodes:         nodes,
		TunnelFactory: factory.New,
		DataDir:       t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	fixedBefore, _ := manager.Snapshot("fixed")
	_, err := manager.DialContext(context.Background(), "rotating", "tcp", "example.com:443")
	if err == nil {
		t.Fatalf("rotating channel dial should fail")
	}
	fixedAfter, _ := manager.Snapshot("fixed")

	if fixedAfter.CurrentNodeID != fixedBefore.CurrentNodeID {
		t.Fatalf("fixed channel node changed from %q to %q", fixedBefore.CurrentNodeID, fixedAfter.CurrentNodeID)
	}
	if fixedAfter.CurrentExitIP != fixedBefore.CurrentExitIP {
		t.Fatalf("fixed channel exit changed from %q to %q", fixedBefore.CurrentExitIP, fixedAfter.CurrentExitIP)
	}
	if fixedAfter.LastError != "" {
		t.Fatalf("fixed channel last error = %q, want empty", fixedAfter.LastError)
	}
}

func TestManagerFixedAutoChannelRestartsCurrentNodeWithoutSwitchingCandidates(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-a", Region: "jp", IP: "198.51.100.10", Speed: 300, Available: true},
		{ID: "jp-b", Region: "jp", IP: "198.51.100.20", Speed: 200, Available: true},
	})
	factory := &recordingFactory{dialErrs: 4}
	manager := NewManager(Config{
		Channels: []config.Channel{{
			ID:            "fixed",
			ListenHost:    "127.0.0.1",
			ListenPort:    3000,
			Region:        "jp",
			RotateMinutes: 0,
			SelectionMode: SelectionAuto,
			Enabled:       true,
		}},
		Nodes:         nodes,
		TunnelFactory: factory.New,
		DataDir:       t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	initial, _ := manager.Snapshot("fixed")
	conn, err := manager.DialContext(context.Background(), "fixed", "tcp", "example.com:443")
	if err != nil {
		t.Fatalf("DialContext: %v", err)
	}
	_ = conn.Close()
	snapshot, _ := manager.Snapshot("fixed")

	if snapshot.CurrentNodeID != initial.CurrentNodeID {
		t.Fatalf("fixed channel switched from %q to %q", initial.CurrentNodeID, snapshot.CurrentNodeID)
	}
	if len(factory.tunnels) != 1 || len(factory.tunnels[0].switchedNodes) != 1 || factory.tunnels[0].switchedNodes[0] != initial.CurrentNodeID {
		t.Fatalf("fixed recovery switches = %#v, want restart current node only", factory.tunnels[0].switchedNodes)
	}
}

func TestManagerRotationAvoidsNodesUsedInLast24Hours(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-a", Region: "jp", Speed: 300, Available: true},
		{ID: "jp-b", Region: "jp", Speed: 200, Available: true},
		{ID: "jp-c", Region: "jp", Speed: 100, Available: true},
	})
	history := newFakeHistory()
	history.recent["jp-3000"] = map[string]time.Time{"jp-b": time.Now().Add(-time.Hour)}
	factory := &recordingFactory{}
	manager := NewManager(Config{
		Channels: []config.Channel{{
			ID:            "jp-3000",
			ListenHost:    "127.0.0.1",
			ListenPort:    3000,
			Region:        "jp",
			RotateMinutes: 10,
			SelectionMode: SelectionAuto,
			Enabled:       true,
		}},
		Nodes:         nodes,
		TunnelFactory: factory.New,
		History:       history,
		DataDir:       t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	if err := manager.RotateNow(context.Background(), "jp-3000"); err != nil {
		t.Fatalf("RotateNow: %v", err)
	}

	snapshot, _ := manager.Snapshot("jp-3000")
	if snapshot.CurrentNodeID != "jp-c" {
		t.Fatalf("current node = %q, want jp-c because jp-a is current and jp-b was used recently", snapshot.CurrentNodeID)
	}
}

func TestManagerRotationAvoidsUnknownUDPWhenAvailableNodeExists(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-current", Region: "jp", Speed: 300, Available: true, ProbeStatus: "available"},
		{ID: "jp-unknown", Region: "jp", Speed: 5000, Available: true, ProbeStatus: "unknown"},
		{ID: "jp-available", Region: "jp", Speed: 100, Available: true, ProbeStatus: "available"},
	})
	factory := &recordingFactory{}
	manager := NewManager(Config{
		Channels: []config.Channel{{
			ID:            "jp-3000",
			ListenHost:    "127.0.0.1",
			ListenPort:    3000,
			Region:        "jp",
			RotateMinutes: 10,
			SelectionMode: SelectionAuto,
			Enabled:       true,
		}},
		Nodes:         nodes,
		TunnelFactory: factory.New,
		DataDir:       t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	if err := manager.RotateNow(context.Background(), "jp-3000"); err != nil {
		t.Fatalf("RotateNow: %v", err)
	}

	snapshot, _ := manager.Snapshot("jp-3000")
	if snapshot.CurrentNodeID != "jp-available" {
		t.Fatalf("current node = %q, want available node over unknown", snapshot.CurrentNodeID)
	}
}

func TestManagerSnapshotTracksRotationMetadata(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-a", Region: "jp", IP: "198.51.100.10", Speed: 300, Available: true},
		{ID: "jp-b", Region: "jp", IP: "198.51.100.20", Speed: 200, Available: true},
	})
	factory := &recordingFactory{}
	manager := NewManager(Config{
		Channels: []config.Channel{{
			ID:            "jp-3000",
			ListenHost:    "127.0.0.1",
			ListenPort:    3000,
			Region:        "jp",
			RotateMinutes: 10,
			SelectionMode: SelectionAuto,
			Enabled:       true,
		}},
		Nodes:         nodes,
		TunnelFactory: factory.New,
		DataDir:       t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	initial, _ := manager.Snapshot("jp-3000")
	if initial.CurrentExitIP != "198.51.100.10" {
		t.Fatalf("current exit ip = %q, want initial node ip", initial.CurrentExitIP)
	}
	if initial.LastExitIP != "" {
		t.Fatalf("last exit ip = %q, want empty before first rotation", initial.LastExitIP)
	}
	if initial.NextRotationAt.IsZero() {
		t.Fatalf("next rotation time should be set for auto channel")
	}

	if err := manager.RotateNow(context.Background(), "jp-3000"); err != nil {
		t.Fatalf("RotateNow: %v", err)
	}

	snapshot, _ := manager.Snapshot("jp-3000")
	if snapshot.LastExitIP != "198.51.100.10" {
		t.Fatalf("last exit ip = %q, want previous node ip", snapshot.LastExitIP)
	}
	if snapshot.CurrentExitIP != "198.51.100.20" {
		t.Fatalf("current exit ip = %q, want rotated node ip", snapshot.CurrentExitIP)
	}
	if snapshot.LastRotationAt.IsZero() {
		t.Fatalf("last rotation time should be set after rotation")
	}
	wantNext := snapshot.LastRotationAt.Add(10 * time.Minute)
	if !snapshot.NextRotationAt.Equal(wantNext) {
		t.Fatalf("next rotation = %v, want %v", snapshot.NextRotationAt, wantNext)
	}
}

func TestManagerPrefersDeepTestSuccessDuringRotation(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-a", Region: "jp", Speed: 300, Available: true},
		{ID: "jp-b", Region: "jp", Speed: 100, Available: true},
		{ID: "jp-c", Region: "jp", Speed: 200, Available: true},
	})
	history := newFakeHistory()
	history.results["jp-b"] = deeptest.Result{NodeID: "jp-b", Status: deeptest.StatusSuccess, ConnectMS: 120}
	history.results["jp-c"] = deeptest.Result{NodeID: "jp-c", Status: deeptest.StatusFailed, ConnectMS: 20}
	factory := &recordingFactory{}
	manager := NewManager(Config{
		Channels: []config.Channel{{
			ID:            "jp-3000",
			ListenHost:    "127.0.0.1",
			ListenPort:    3000,
			Region:        "jp",
			RotateMinutes: 10,
			SelectionMode: SelectionAuto,
			Enabled:       true,
		}},
		Nodes:         nodes,
		TunnelFactory: factory.New,
		History:       history,
		DataDir:       t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	if err := manager.RotateNow(context.Background(), "jp-3000"); err != nil {
		t.Fatalf("RotateNow: %v", err)
	}

	snapshot, _ := manager.Snapshot("jp-3000")
	if snapshot.CurrentNodeID != "jp-b" {
		t.Fatalf("current node = %q, want deep-test-success jp-b", snapshot.CurrentNodeID)
	}
}

func TestManagerRefreshesNodesBeforeRotation(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-a", Region: "jp", Speed: 300, Available: true},
		{ID: "jp-b", Region: "jp", Speed: 200, Available: true},
	})
	refreshed := false
	factory := &recordingFactory{}
	manager := NewManager(Config{
		Channels: []config.Channel{{
			ID:            "jp-3000",
			ListenHost:    "127.0.0.1",
			ListenPort:    3000,
			Region:        "jp",
			RotateMinutes: 10,
			SelectionMode: SelectionAuto,
			Enabled:       true,
		}},
		Nodes:         nodes,
		TunnelFactory: factory.New,
		RefreshNodes: func(ctx context.Context) error {
			refreshed = true
			nodes.Replace([]node.Node{
				{ID: "jp-a", Region: "jp", Speed: 300, Available: true},
				{ID: "jp-fresh", Region: "jp", Speed: 900, Available: true},
			})
			return nil
		},
		DataDir: t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	if err := manager.RotateNow(context.Background(), "jp-3000"); err != nil {
		t.Fatalf("RotateNow: %v", err)
	}

	snapshot, _ := manager.Snapshot("jp-3000")
	if !refreshed {
		t.Fatalf("RefreshNodes was not called")
	}
	if snapshot.CurrentNodeID != "jp-fresh" {
		t.Fatalf("current node = %q, want refreshed jp-fresh", snapshot.CurrentNodeID)
	}
}

func TestManagerDialReturnsErrorWhenRetriesAndRotationFail(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-a", Region: "jp", Speed: 100, Available: true},
		{ID: "jp-b", Region: "jp", Speed: 200, Available: true},
	})
	factory := &recordingFactory{dialErrs: 10, switchErr: fmt.Errorf("switch failed")}
	manager := NewManager(Config{
		Channels: []config.Channel{{
			ID:            "jp-3000",
			ListenHost:    "127.0.0.1",
			ListenPort:    3000,
			Region:        "jp",
			SelectionMode: SelectionAuto,
			Enabled:       true,
		}},
		Nodes:         nodes,
		TunnelFactory: factory.New,
		DataDir:       t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	_, err := manager.DialContext(context.Background(), "jp-3000", "tcp", "example.com:443")
	if err == nil {
		t.Fatalf("DialContext should fail")
	}
	if !strings.Contains(err.Error(), "after 3 retries") {
		t.Fatalf("error = %q, want retry context", err)
	}
	snapshot, _ := manager.Snapshot("jp-3000")
	if snapshot.CurrentNodeID != "jp-b" {
		t.Fatalf("current node = %q, want jp-b after failed rotation", snapshot.CurrentNodeID)
	}
	if snapshot.LastError == "" {
		t.Fatalf("last error should be recorded")
	}
}

type recordingFactory struct {
	tunnels          []*recordingTunnel
	switchErr        error
	dialErrs         int
	dialErrsByTunnel []int
	dialErrByNode    map[string]error
	dialErrByChannel map[string]error
}

func (f *recordingFactory) New(name string) tunnel.Tunnel {
	dialErrs := f.dialErrs
	if len(f.dialErrsByTunnel) > len(f.tunnels) {
		dialErrs = f.dialErrsByTunnel[len(f.tunnels)]
	}
	tun := &recordingTunnel{name: name, switchErr: f.switchErr, dialErrs: dialErrs, dialErrByNode: f.dialErrByNode, dialErrByChannel: f.dialErrByChannel}
	f.tunnels = append(f.tunnels, tun)
	return tun
}

type recordingTunnel struct {
	name             string
	startedNode      node.Node
	switchedNode     node.Node
	switchedNodes    []string
	switchErr        error
	dialErrs         int
	dialErrByNode    map[string]error
	dialErrByChannel map[string]error
	dialCount        int
	stopped          bool
}

func (t *recordingTunnel) Start(ctx context.Context, n node.Node, opts tunnel.Options) error {
	t.startedNode = n
	return nil
}

func (t *recordingTunnel) Stop(ctx context.Context) error {
	t.stopped = true
	return nil
}

func (t *recordingTunnel) Switch(ctx context.Context, n node.Node) error {
	if t.switchErr != nil {
		return t.switchErr
	}
	t.startedNode = n
	t.switchedNode = n
	t.switchedNodes = append(t.switchedNodes, n.ID)
	return nil
}

func (t *recordingTunnel) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	t.dialCount++
	if err := t.dialErrByChannel[t.name]; err != nil {
		return nil, err
	}
	if err := t.dialErrByNode[t.startedNode.ID]; err != nil {
		return nil, err
	}
	if t.dialCount <= t.dialErrs {
		return nil, fmt.Errorf("dial failed")
	}
	client, server := net.Pipe()
	_ = server.Close()
	return client, nil
}

func (t *recordingTunnel) Status() tunnel.Status {
	return tunnel.Status{Name: t.name, NodeID: t.startedNode.ID, Ready: true}
}

type fakeHistory struct {
	recent  map[string]map[string]time.Time
	results map[string]deeptest.Result
	uses    []NodeUse
}

func newFakeHistory() *fakeHistory {
	return &fakeHistory{recent: map[string]map[string]time.Time{}, results: map[string]deeptest.Result{}}
}

func (h *fakeHistory) RecentNodeIDsForChannel(ctx context.Context, channelID string, since time.Time) (map[string]time.Time, error) {
	out := map[string]time.Time{}
	for nodeID, usedAt := range h.recent[channelID] {
		if usedAt.After(since) || usedAt.Equal(since) {
			out[nodeID] = usedAt
		}
	}
	return out, nil
}

func (h *fakeHistory) DeepTestResults(ctx context.Context) (map[string]deeptest.Result, error) {
	out := map[string]deeptest.Result{}
	for nodeID, result := range h.results {
		out[nodeID] = result
	}
	return out, nil
}

func (h *fakeHistory) RecordChannelNodeUse(ctx context.Context, use NodeUse) error {
	h.uses = append(h.uses, use)
	return nil
}
