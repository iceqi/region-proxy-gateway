package channel

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/iceqi/region-proxy-gateway/internal/config"
	"github.com/iceqi/region-proxy-gateway/internal/node"
	"github.com/iceqi/region-proxy-gateway/internal/tunnel"
)

func TestManagerStartsAutoChannelWithBestRegionalNode(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-slow", Region: "jp", Hostname: "slow", Speed: 100, LatencyMS: 10, Available: true},
		{ID: "jp-fast", Region: "jp", Hostname: "fast", Speed: 1000, LatencyMS: 80, Available: true},
		{ID: "us-fast", Region: "us", Hostname: "us", Speed: 5000, LatencyMS: 10, Available: true},
	})
	factory := &recordingFactory{}
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

	snapshots := manager.Snapshots()
	if len(snapshots) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(snapshots))
	}
	if snapshots[0].CurrentNodeID != "jp-fast" {
		t.Fatalf("current node = %q, want jp-fast", snapshots[0].CurrentNodeID)
	}
	if factory.tunnels[0].startedNode.ID != "jp-fast" {
		t.Fatalf("started node = %q, want jp-fast", factory.tunnels[0].startedNode.ID)
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
	tunnels   []*recordingTunnel
	switchErr error
	dialErrs  int
}

func (f *recordingFactory) New(name string) tunnel.Tunnel {
	tun := &recordingTunnel{name: name, switchErr: f.switchErr, dialErrs: f.dialErrs}
	f.tunnels = append(f.tunnels, tun)
	return tun
}

type recordingTunnel struct {
	name         string
	startedNode  node.Node
	switchedNode node.Node
	switchErr    error
	dialErrs     int
	dialCount    int
}

func (t *recordingTunnel) Start(ctx context.Context, n node.Node, opts tunnel.Options) error {
	t.startedNode = n
	return nil
}

func (t *recordingTunnel) Stop(ctx context.Context) error {
	return nil
}

func (t *recordingTunnel) Switch(ctx context.Context, n node.Node) error {
	if t.switchErr != nil {
		return t.switchErr
	}
	t.switchedNode = n
	return nil
}

func (t *recordingTunnel) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	t.dialCount++
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
