package nodecheck

import (
	"context"
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
		return 18, nil
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
	if got.LatencyMS != 18 {
		t.Fatalf("latency = %d, want ping latency 18", got.LatencyMS)
	}
	if got.LastTestedAt.IsZero() {
		t.Fatalf("expected LastTestedAt")
	}
}

func TestCheckerMarksFailure(t *testing.T) {
	checker := Checker{Timeout: 10 * time.Millisecond}
	checker.Ping = func(ctx context.Context, host string, timeout time.Duration) (int, error) {
		return 0, context.DeadlineExceeded
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
