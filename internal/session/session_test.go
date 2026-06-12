package session

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/node"
	"github.com/iceqi/region-proxy-gateway/internal/strategy"
	"github.com/iceqi/region-proxy-gateway/internal/tunnel"
)

type recordingTunnel struct {
	mu          sync.Mutex
	startedWith node.Node
	options     tunnel.Options
	switchedTo  node.Node
	startErr    error
	switchErr   error
	switchBlock chan struct{}
	switchCount int32
}

func (t *recordingTunnel) Start(ctx context.Context, n node.Node, opts tunnel.Options) error {
	if t.startErr != nil {
		return t.startErr
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.startedWith = n
	t.options = opts
	return nil
}

func (t *recordingTunnel) Stop(ctx context.Context) error { return nil }

func (t *recordingTunnel) Switch(ctx context.Context, n node.Node) error {
	atomic.AddInt32(&t.switchCount, 1)
	if t.switchBlock != nil {
		<-t.switchBlock
	}
	if t.switchErr != nil {
		return t.switchErr
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.switchedTo = n
	return nil
}

func (t *recordingTunnel) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return nil, nil
}

func (t *recordingTunnel) Status() tunnel.Status { return tunnel.Status{} }

func TestGetOrCreateReusesSessionForStrategy(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{{ID: "jp-a", Region: "jp", LatencyMS: 10, Available: true}})
	var created int
	var rec *recordingTunnel
	manager := NewManager(nodes, 2, func(key string) tunnel.Tunnel {
		created++
		rec = &recordingTunnel{}
		return rec
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

	if first.Strategy.Key() != second.Strategy.Key() {
		t.Fatalf("expected same session key, got %q and %q", first.Strategy.Key(), second.Strategy.Key())
	}
	if manager.ActiveCount() != 1 {
		t.Fatalf("expected one active session, got %d", manager.ActiveCount())
	}
	if created != 1 {
		t.Fatalf("factory calls = %d, want 1", created)
	}
	if !second.LastUsedAt.After(second.CreatedAt) {
		t.Fatalf("expected reuse to update LastUsedAt")
	}
	if rec.startedWith.ID != "jp-a" {
		t.Fatalf("tunnel started with %q, want jp-a", rec.startedWith.ID)
	}
	if rec.options.Name != "jp-15" || rec.options.DeviceName != "rpg0" {
		t.Fatalf("tunnel options = %+v, want name jp-15 and device rpg0", rec.options)
	}
}

func TestGetOrCreateAllocatesDistinctDeviceNames(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-a", Region: "jp", LatencyMS: 10, Available: true},
		{ID: "us-a", Region: "us", LatencyMS: 10, Available: true},
	})
	tunnels := map[string]*recordingTunnel{}
	manager := NewManager(nodes, 2, func(key string) tunnel.Tunnel {
		tun := &recordingTunnel{}
		tunnels[key] = tun
		return tun
	})
	ctx := context.Background()

	if _, err := manager.GetOrCreate(ctx, strategy.Strategy{Region: "jp", RotateMinutes: 15}); err != nil {
		t.Fatalf("GetOrCreate jp: %v", err)
	}
	if _, err := manager.GetOrCreate(ctx, strategy.Strategy{Region: "us", RotateMinutes: 0}); err != nil {
		t.Fatalf("GetOrCreate us: %v", err)
	}

	if tunnels["jp-15"].options.DeviceName != "rpg0" {
		t.Fatalf("jp device = %q, want rpg0", tunnels["jp-15"].options.DeviceName)
	}
	if tunnels["us-0"].options.DeviceName != "rpg1" {
		t.Fatalf("us device = %q, want rpg1", tunnels["us-0"].options.DeviceName)
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
	sessions := manager.List()
	if len(sessions) != 1 {
		t.Fatalf("session count = %d, want 1", len(sessions))
	}
	updated := sessions[0]

	if updated.Node.ID != "jp-b" {
		t.Fatalf("expected switched node jp-b, got %q", updated.Node.ID)
	}
	rec := sess.Tunnel.(*recordingTunnel)
	if rec.switchedTo.ID != "jp-b" {
		t.Fatalf("expected tunnel switched to jp-b, got %q", rec.switchedTo.ID)
	}
	if !updated.LastUsedAt.After(updated.CreatedAt) {
		t.Fatalf("expected SwitchNow to update LastUsedAt")
	}
}

func TestGetOrCreateReturnsErrorWhenNoNodeAvailable(t *testing.T) {
	nodes := node.NewStore()
	manager := NewManager(nodes, 2, func(key string) tunnel.Tunnel {
		return &recordingTunnel{}
	})

	if _, err := manager.GetOrCreate(context.Background(), strategy.Strategy{Region: "jp", RotateMinutes: 15}); err == nil {
		t.Fatalf("expected no available node error")
	}
}

func TestGetOrCreateReturnsErrorWhenFactoryReturnsNil(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{{ID: "jp-a", Region: "jp", LatencyMS: 10, Available: true}})
	manager := NewManager(nodes, 2, func(key string) tunnel.Tunnel {
		return nil
	})

	if _, err := manager.GetOrCreate(context.Background(), strategy.Strategy{Region: "jp", RotateMinutes: 15}); err == nil {
		t.Fatalf("expected nil factory error")
	}
}

func TestGetOrCreateReturnsStartError(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{{ID: "jp-a", Region: "jp", LatencyMS: 10, Available: true}})
	wantErr := errors.New("start failed")
	manager := NewManager(nodes, 2, func(key string) tunnel.Tunnel {
		return &recordingTunnel{startErr: wantErr}
	})

	if _, err := manager.GetOrCreate(context.Background(), strategy.Strategy{Region: "jp", RotateMinutes: 15}); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want start failed", err)
	}
}

func TestGetOrCreateCleansCreatingAfterStartError(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{{ID: "jp-a", Region: "jp", LatencyMS: 10, Available: true}})
	var calls int
	manager := NewManager(nodes, 2, func(key string) tunnel.Tunnel {
		calls++
		if calls == 1 {
			return &recordingTunnel{startErr: errors.New("start failed")}
		}
		return &recordingTunnel{}
	})
	strat := strategy.Strategy{Region: "jp", RotateMinutes: 15}
	if _, err := manager.GetOrCreate(context.Background(), strat); err == nil {
		t.Fatalf("expected first create to fail")
	}
	if _, err := manager.GetOrCreate(context.Background(), strat); err != nil {
		t.Fatalf("expected retry to succeed, got %v", err)
	}
}

func TestConcurrentCreateCountsCreatingAgainstMaxSessions(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-a", Region: "jp", LatencyMS: 10, Available: true},
		{ID: "us-a", Region: "us", LatencyMS: 10, Available: true},
	})
	started := make(chan struct{})
	release := make(chan struct{})
	manager := NewManager(nodes, 1, func(key string) tunnel.Tunnel {
		return &blockingStartTunnel{started: started, release: release}
	})

	errs := make(chan error, 2)
	go func() {
		_, err := manager.GetOrCreate(context.Background(), strategy.Strategy{Region: "jp", RotateMinutes: 15})
		errs <- err
	}()
	<-started

	go func() {
		_, err := manager.GetOrCreate(context.Background(), strategy.Strategy{Region: "us", RotateMinutes: 0})
		errs <- err
	}()

	secondErr := <-errs
	if secondErr == nil {
		t.Fatalf("expected second create for a different key to fail while max sessions are reserved")
	}
	close(release)
	firstErr := <-errs
	if firstErr != nil {
		t.Fatalf("expected first create to succeed, got %v", firstErr)
	}
	if manager.ActiveCount() != 1 {
		t.Fatalf("ActiveCount = %d, want 1", manager.ActiveCount())
	}
}

func TestConcurrentSameKeyCreateWaitsForInFlightCreate(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{{ID: "jp-a", Region: "jp", LatencyMS: 10, Available: true}})
	started := make(chan struct{})
	release := make(chan struct{})
	var factoryCalls int32
	manager := NewManager(nodes, 2, func(key string) tunnel.Tunnel {
		atomic.AddInt32(&factoryCalls, 1)
		return &blockingStartTunnel{started: started, release: release}
	})
	strat := strategy.Strategy{Region: "jp", RotateMinutes: 15}

	results := make(chan struct {
		sess sessionResult
		err  error
	}, 2)
	go func() {
		sess, err := manager.GetOrCreate(context.Background(), strat)
		results <- struct {
			sess sessionResult
			err  error
		}{sess: sessionResult{key: sess.Strategy.Key()}, err: err}
	}()
	<-started

	go func() {
		sess, err := manager.GetOrCreate(context.Background(), strat)
		results <- struct {
			sess sessionResult
			err  error
		}{sess: sessionResult{key: sess.Strategy.Key()}, err: err}
	}()
	select {
	case result := <-results:
		t.Fatalf("second create returned before first create completed: %v", result.err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	for i := 0; i < 2; i++ {
		result := <-results
		if result.err != nil {
			t.Fatalf("GetOrCreate returned error: %v", result.err)
		}
		if result.sess.key != "jp-15" {
			t.Fatalf("session key = %q, want jp-15", result.sess.key)
		}
	}
	if factoryCalls != 1 {
		t.Fatalf("factory calls = %d, want 1", factoryCalls)
	}
}

type sessionResult struct {
	key string
}

func TestSwitchNowReturnsErrorWhenNoAlternativeNode(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{{ID: "jp-a", Region: "jp", LatencyMS: 10, Available: true}})
	manager := NewManager(nodes, 2, func(key string) tunnel.Tunnel {
		return &recordingTunnel{}
	})
	strat := strategy.Strategy{Region: "jp", RotateMinutes: 15}
	if _, err := manager.GetOrCreate(context.Background(), strat); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	if err := manager.SwitchNow(context.Background(), strat.Key()); err == nil {
		t.Fatalf("expected no alternative node error")
	}
}

func TestSwitchNowReturnsTunnelError(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-a", Region: "jp", LatencyMS: 10, Available: true},
		{ID: "jp-b", Region: "jp", LatencyMS: 20, Available: true},
	})
	wantErr := errors.New("switch failed")
	manager := NewManager(nodes, 2, func(key string) tunnel.Tunnel {
		return &recordingTunnel{switchErr: wantErr}
	})
	strat := strategy.Strategy{Region: "jp", RotateMinutes: 15}
	if _, err := manager.GetOrCreate(context.Background(), strat); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	if err := manager.SwitchNow(context.Background(), strat.Key()); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want switch failed", err)
	}
}

func TestConcurrentSwitchNowRejectsSecondSwitch(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-a", Region: "jp", LatencyMS: 10, Available: true},
		{ID: "jp-b", Region: "jp", LatencyMS: 20, Available: true},
	})
	block := make(chan struct{})
	rec := &recordingTunnel{switchBlock: block}
	manager := NewManager(nodes, 2, func(key string) tunnel.Tunnel {
		return rec
	})
	strat := strategy.Strategy{Region: "jp", RotateMinutes: 15}
	if _, err := manager.GetOrCreate(context.Background(), strat); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	errs := make(chan error, 2)
	go func() {
		errs <- manager.SwitchNow(context.Background(), strat.Key())
	}()
	for atomic.LoadInt32(&rec.switchCount) == 0 {
		time.Sleep(time.Millisecond)
	}
	go func() {
		errs <- manager.SwitchNow(context.Background(), strat.Key())
	}()
	secondErr := <-errs
	if secondErr == nil {
		t.Fatalf("expected second switch to be rejected while first is switching")
	}
	close(block)
	if firstErr := <-errs; firstErr != nil {
		t.Fatalf("expected first switch to succeed, got %v", firstErr)
	}
}

func TestSessionJSONOmitsTunnel(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{{ID: "jp-a", Region: "jp", LatencyMS: 10, Available: true}})
	manager := NewManager(nodes, 2, func(key string) tunnel.Tunnel {
		return &recordingTunnel{}
	})
	sess, err := manager.GetOrCreate(context.Background(), strategy.Strategy{Region: "jp", RotateMinutes: 15})
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	raw, err := json.Marshal(sess)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(raw) == "" || json.Valid(raw) == false {
		t.Fatalf("invalid json: %s", raw)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, ok := body["Tunnel"]; ok {
		t.Fatalf("json should omit Tunnel field: %s", raw)
	}
	if _, ok := body["tunnel"]; ok {
		t.Fatalf("json should omit tunnel field: %s", raw)
	}
}

type blockingStartTunnel struct {
	started chan struct{}
	release chan struct{}
}

func (t *blockingStartTunnel) Start(ctx context.Context, n node.Node, opts tunnel.Options) error {
	close(t.started)
	<-t.release
	return nil
}

func (t *blockingStartTunnel) Stop(ctx context.Context) error { return nil }

func (t *blockingStartTunnel) Switch(ctx context.Context, n node.Node) error { return nil }

func (t *blockingStartTunnel) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return nil, nil
}

func (t *blockingStartTunnel) Status() tunnel.Status { return tunnel.Status{} }
