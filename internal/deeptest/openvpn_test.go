package deeptest

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/node"
	"github.com/iceqi/region-proxy-gateway/internal/tunnel"
)

func TestOpenVPNTesterUsesStableUniqueDeviceNames(t *testing.T) {
	first := testDeviceName("node-a")
	second := testDeviceName("node-b")
	if first == second {
		t.Fatalf("device names should differ for different nodes")
	}
	if len(first) > 15 || len(second) > 15 {
		t.Fatalf("device names must fit linux interface limit: %q %q", first, second)
	}
}

func TestOpenVPNTesterRecordsSuccessAndStopsTunnel(t *testing.T) {
	starter := &testProcessStarter{}
	dialer := &httpResponseDialer{body: `{"ip":"203.0.113.77","country":"Japan"}`}
	tester := OpenVPNTester{
		DataDir:      t.TempDir(),
		Command:      "/usr/sbin/openvpn",
		Starter:      starter,
		DeviceDialer: dialer,
		HTTPClient:   &http.Client{Timeout: time.Second},
		ExitIPURL:    "http://ipinfo.test/json",
	}

	result := tester.Test(context.Background(), node.Node{ID: "jp-1", OpenVPN: "client\n"})

	if result.Status != StatusSuccess {
		t.Fatalf("result = %+v, want success", result)
	}
	if result.ExitIP != "203.0.113.77" || result.ExitCountry != "Japan" {
		t.Fatalf("exit = %s/%s, want parsed ip country", result.ExitIP, result.ExitCountry)
	}
	if result.ConnectMS <= 0 {
		t.Fatalf("connect ms = %d, want positive", result.ConnectMS)
	}
	if len(starter.commands) != 1 || starter.commands[0][0] != "/usr/sbin/openvpn" {
		t.Fatalf("commands = %+v, want openvpn start", starter.commands)
	}
	if !starter.process.terminated {
		t.Fatalf("temporary openvpn process should be terminated")
	}
}

func TestOpenVPNTesterFailsWhenExitIPCheckFails(t *testing.T) {
	starter := &testProcessStarter{}
	tester := OpenVPNTester{
		DataDir:      t.TempDir(),
		Starter:      starter,
		DeviceDialer: &httpResponseDialer{err: errors.New("dial failed")},
		HTTPClient:   &http.Client{Timeout: time.Second},
		ExitIPURL:    "http://ipinfo.test/json",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	result := tester.Test(ctx, node.Node{ID: "jp-1", OpenVPN: "client\n"})

	if result.Status != StatusFailed {
		t.Fatalf("result = %+v, want failed", result)
	}
	if !strings.Contains(result.FailReason, "exit IP check failed") {
		t.Fatalf("fail reason = %q, want exit IP check failed", result.FailReason)
	}
	if !starter.process.terminated {
		t.Fatalf("temporary openvpn process should be terminated after failure")
	}
}

func TestOpenVPNTesterRetriesExitIPCheckUntilTunnelWorks(t *testing.T) {
	starter := &testProcessStarter{}
	dialer := &flakyHTTPDialer{
		failures: 2,
		success:  &httpResponseDialer{body: `{"ip":"203.0.113.88"}`},
	}
	tester := OpenVPNTester{
		DataDir:       t.TempDir(),
		Starter:       starter,
		DeviceDialer:  dialer,
		HTTPClient:    &http.Client{Timeout: time.Second},
		ExitIPURL:     "http://ipinfo.test/json",
		RetryInterval: time.Millisecond,
	}

	result := tester.Test(context.Background(), node.Node{ID: "jp-1", OpenVPN: "client\n"})

	if result.Status != StatusSuccess || result.ExitIP != "203.0.113.88" {
		t.Fatalf("result = %+v, want success after retries", result)
	}
	if dialer.calls != 3 {
		t.Fatalf("dial calls = %d, want 3", dialer.calls)
	}
}

func TestOpenVPNTesterRejectsMissingConfig(t *testing.T) {
	tester := OpenVPNTester{DataDir: t.TempDir(), Starter: &testProcessStarter{}, DeviceDialer: &httpResponseDialer{}}

	result := tester.Test(context.Background(), node.Node{ID: "jp-1"})

	if result.Status != StatusFailed || !strings.Contains(result.FailReason, "empty openvpn config") {
		t.Fatalf("result = %+v, want missing config failure", result)
	}
}

type testProcessStarter struct {
	commands [][]string
	process  *testProcess
}

func (s *testProcessStarter) Start(ctx context.Context, command []string) (tunnel.OpenVPNProcess, error) {
	s.commands = append(s.commands, append([]string(nil), command...))
	s.process = &testProcess{}
	return s.process, nil
}

type testProcess struct {
	terminated bool
	killed     bool
}

func (p *testProcess) PID() int { return 5678 }
func (p *testProcess) Wait() error {
	for !p.terminated && !p.killed {
		time.Sleep(time.Millisecond)
	}
	return nil
}
func (p *testProcess) Terminate() error {
	p.terminated = true
	return nil
}
func (p *testProcess) Kill() error {
	p.killed = true
	return nil
}

type flakyHTTPDialer struct {
	failures int
	calls    int
	success  *httpResponseDialer
}

func (d *flakyHTTPDialer) DialContext(ctx context.Context, deviceName, network, address string) (net.Conn, error) {
	d.calls++
	if d.calls <= d.failures {
		return nil, errors.New("temporary tunnel not ready")
	}
	return d.success.DialContext(ctx, deviceName, network, address)
}

type httpResponseDialer struct {
	body string
	err  error
}

func (d *httpResponseDialer) DialContext(ctx context.Context, deviceName, network, address string) (net.Conn, error) {
	if d.err != nil {
		return nil, d.err
	}
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		buf := make([]byte, 4096)
		_, _ = server.Read(buf)
		body := d.body
		if body == "" {
			body = `{"ip":"203.0.113.10"}`
		}
		_, _ = server.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: " + stringLen(body) + "\r\n\r\n" + body))
	}()
	return client, nil
}

func stringLen(value string) string {
	return strconv.Itoa(len(value))
}
