package tunnel

import (
	"context"
	"testing"

	"github.com/iceqi/region-proxy-gateway/internal/node"
)

func TestFakeStartSwitchStopStatus(t *testing.T) {
	ctx := context.Background()
	f := NewFake("initial")

	if status := f.Status(); status.Name != "initial" || status.Ready {
		t.Fatalf("initial status = %+v, want name initial and not ready", status)
	}

	if err := f.Start(ctx, node.Node{ID: "node-a"}, Options{Name: "fake"}); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	status := f.Status()
	if status.Name != "fake" || status.NodeID != "node-a" || !status.Ready || status.StartedAt.IsZero() {
		t.Fatalf("status after Start = %+v, want started node-a", status)
	}

	if err := f.Switch(ctx, node.Node{ID: "node-b"}); err != nil {
		t.Fatalf("Switch returned error: %v", err)
	}

	status = f.Status()
	if status.NodeID != "node-b" || !status.Ready || status.Error != "" || status.StartedAt.IsZero() {
		t.Fatalf("status after Switch = %+v, want ready node-b without error", status)
	}

	if err := f.Stop(ctx); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}

	if status := f.Status(); status.Ready {
		t.Fatalf("status after Stop = %+v, want not ready", status)
	}
}

func TestFakeImplementsTunnel(t *testing.T) {
	var _ Tunnel = NewFake("fake")
}
