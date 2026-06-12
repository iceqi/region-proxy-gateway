package session

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/node"
	"github.com/iceqi/region-proxy-gateway/internal/strategy"
	"github.com/iceqi/region-proxy-gateway/internal/tunnel"
)

type recordingTunnel struct {
	startedWith node.Node
	switchedTo  node.Node
}

func (t *recordingTunnel) Start(ctx context.Context, n node.Node, opts tunnel.Options) error {
	t.startedWith = n
	return nil
}

func (t *recordingTunnel) Stop(ctx context.Context) error { return nil }

func (t *recordingTunnel) Switch(ctx context.Context, n node.Node) error {
	t.switchedTo = n
	return nil
}

func (t *recordingTunnel) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return nil, nil
}

func (t *recordingTunnel) Status() tunnel.Status { return tunnel.Status{} }

func TestGetOrCreateReusesSameSessionForStrategy(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{{ID: "jp-a", Region: "jp", LatencyMS: 10, Available: true}})
	manager := NewManager(nodes, 2, func(key string) tunnel.Tunnel {
		return &recordingTunnel{}
	})
	ctx := context.Background()
	strat := strategy.Strategy{Region: "jp", RotateMinutes: 15}

	first, err := manager.GetOrCreate(ctx, strat)
	if err != nil {
		t.Fatalf("GetOrCreate first: %v", err)
	}
	time.Sleep(time.Millisecond)
	second, err := manager.GetOrCreate(ctx, strat)
	if err != nil {
		t.Fatalf("GetOrCreate second: %v", err)
	}

	if first != second {
		t.Fatalf("expected same session pointer, got %p and %p", first, second)
	}
	if manager.ActiveCount() != 1 {
		t.Fatalf("expected one active session, got %d", manager.ActiveCount())
	}
	if !second.LastUsedAt.After(second.CreatedAt) {
		t.Fatalf("expected reuse to update LastUsedAt")
	}
}

func TestGetOrCreateReturnsErrorWhenMaxSessionsExceeded(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-a", Region: "jp", LatencyMS: 10, Available: true},
		{ID: "us-a", Region: "us", LatencyMS: 10, Available: true},
	})
	manager := NewManager(nodes, 1, func(key string) tunnel.Tunnel {
		return &recordingTunnel{}
	})
	ctx := context.Background()

	if _, err := manager.GetOrCreate(ctx, strategy.Strategy{Region: "jp", RotateMinutes: 15}); err != nil {
		t.Fatalf("GetOrCreate first: %v", err)
	}
	if _, err := manager.GetOrCreate(ctx, strategy.Strategy{Region: "us", RotateMinutes: 15}); err == nil {
		t.Fatalf("expected max sessions error")
	}
}

func TestSwitchNowChangesToAlternativeNodeInSameRegion(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-a", Region: "jp", LatencyMS: 10, Available: true},
		{ID: "jp-b", Region: "jp", LatencyMS: 20, Available: true},
	})
	manager := NewManager(nodes, 2, func(key string) tunnel.Tunnel {
		return &recordingTunnel{}
	})
	ctx := context.Background()
	strat := strategy.Strategy{Region: "jp", RotateMinutes: 15}

	sess, err := manager.GetOrCreate(ctx, strat)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if sess.Node.ID != "jp-a" {
		t.Fatalf("expected initial best node jp-a, got %q", sess.Node.ID)
	}
	if err := manager.SwitchNow(ctx, strat.Key()); err != nil {
		t.Fatalf("SwitchNow: %v", err)
	}

	if sess.Node.ID != "jp-b" {
		t.Fatalf("expected switched node jp-b, got %q", sess.Node.ID)
	}
	rec := sess.Tunnel.(*recordingTunnel)
	if rec.switchedTo.ID != "jp-b" {
		t.Fatalf("expected tunnel switched to jp-b, got %q", rec.switchedTo.ID)
	}
}
