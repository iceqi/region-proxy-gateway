package channel

import (
	"context"
	"net"
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

type recordingFactory struct {
	tunnels []*recordingTunnel
}

func (f *recordingFactory) New(name string) tunnel.Tunnel {
	tun := &recordingTunnel{name: name}
	f.tunnels = append(f.tunnels, tun)
	return tun
}

type recordingTunnel struct {
	name         string
	startedNode  node.Node
	switchedNode node.Node
}

func (t *recordingTunnel) Start(ctx context.Context, n node.Node, opts tunnel.Options) error {
	t.startedNode = n
	return nil
}

func (t *recordingTunnel) Stop(ctx context.Context) error {
	return nil
}

func (t *recordingTunnel) Switch(ctx context.Context, n node.Node) error {
	t.switchedNode = n
	return nil
}

func (t *recordingTunnel) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, network, address)
}

func (t *recordingTunnel) Status() tunnel.Status {
	return tunnel.Status{Name: t.name, NodeID: t.startedNode.ID, Ready: true}
}
