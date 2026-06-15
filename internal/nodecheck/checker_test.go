package nodecheck

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/node"
)

func TestCheckerMeasuresTCPConnectLatency(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	checker := Checker{Timeout: time.Second}
	checker.Ping = func(ctx context.Context, host string, timeout time.Duration) (int, error) {
		t.Fatalf("tcp node should not use ping latency")
		return 0, nil
	}
	got := checker.Check(context.Background(), node.Node{
		ID:       "local",
		Hostname: "127.0.0.1",
		Port:     listener.Addr().(*net.TCPAddr).Port,
		Proto:    "tcp",
	})

	if !got.Available {
		t.Fatalf("expected node available, fail reason=%q", got.FailReason)
	}
	if got.LatencyMS <= 0 {
		t.Fatalf("latency = %d, want tcp connect latency", got.LatencyMS)
	}
	if got.LastTestedAt.IsZero() {
		t.Fatalf("expected LastTestedAt")
	}
}

func TestCheckerMarksFailure(t *testing.T) {
	checker := Checker{Timeout: 10 * time.Millisecond}
	checker.Ping = func(ctx context.Context, host string, timeout time.Duration) (int, error) {
		t.Fatalf("tcp node should fail by tcp connect, not ping")
		return 0, nil
	}
	got := checker.Check(context.Background(), node.Node{
		ID:       "bad",
		Hostname: "127.0.0.1",
		Port:     1,
		Proto:    "tcp",
	})

	if got.Available {
		t.Fatalf("expected node unavailable")
	}
	if got.FailReason == "" {
		t.Fatalf("expected fail reason")
	}
	if got.ProbeStatus != "unavailable" {
		t.Fatalf("probe status = %q, want unavailable", got.ProbeStatus)
	}
}

func TestCheckerDeprioritizesUDPNodeWhenPingFails(t *testing.T) {
	checker := Checker{Timeout: time.Second}
	checker.Ping = func(ctx context.Context, host string, timeout time.Duration) (int, error) {
		return 0, errors.New("icmp blocked")
	}

	got := checker.Check(context.Background(), node.Node{
		ID:        "udp",
		IP:        "203.0.113.10",
		Proto:     "udp",
		LatencyMS: 0,
	})

	if !got.Available {
		t.Fatalf("UDP node should stay as fallback candidate when ping fails")
	}
	if got.LatencyMS != 0 {
		t.Fatalf("latency = %d, want unknown 0", got.LatencyMS)
	}
	if got.ProbeStatus != "unknown" {
		t.Fatalf("probe status = %q, want unknown", got.ProbeStatus)
	}
	if got.FailReason != "" {
		t.Fatalf("fail reason = %q, want empty because UDP reachability is unknown, not confirmed failed", got.FailReason)
	}
	if got.ProbeMessage != "udp host unreachable; deprioritized until deep test or successful ping" {
		t.Fatalf("probe message = %q", got.ProbeMessage)
	}
}

func TestCheckerAcceptsUDPNodesWhenPingWorks(t *testing.T) {
	checker := Checker{Timeout: time.Second}
	checker.Ping = func(ctx context.Context, host string, timeout time.Duration) (int, error) {
		return 42, nil
	}

	got := checker.Check(context.Background(), node.Node{
		ID:       "udp",
		Hostname: "127.0.0.1",
		Proto:    "udp",
	})

	if !got.Available {
		t.Fatalf("expected UDP node available with working ping, fail=%q", got.FailReason)
	}
	if got.LatencyMS != 42 {
		t.Fatalf("latency = %d, want 42", got.LatencyMS)
	}
	if got.ProbeStatus != "available" {
		t.Fatalf("probe status = %q, want available", got.ProbeStatus)
	}
}

func TestCheckerPrefersIPWhenHostnameIsNotResolvable(t *testing.T) {
	checker := Checker{Timeout: time.Second}
	var pingHost string
	checker.Ping = func(ctx context.Context, host string, timeout time.Duration) (int, error) {
		pingHost = host
		return 31, nil
	}

	got := checker.Check(context.Background(), node.Node{
		ID:       "vpngate",
		Hostname: "vpn743566583",
		IP:       "203.0.113.45",
		Proto:    "udp",
	})

	if !got.Available {
		t.Fatalf("expected node available, fail=%q", got.FailReason)
	}
	if pingHost != "203.0.113.45" {
		t.Fatalf("ping host = %q, want IP fallback", pingHost)
	}
}
