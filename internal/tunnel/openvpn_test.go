package tunnel

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/node"
)

func TestOpenVPNCommandIncludesCoreOptions(t *testing.T) {
	got := OpenVPNCommand("/usr/sbin/openvpn", "/tmp/client.ovpn", "tun-test")

	want := []string{
		"/usr/sbin/openvpn",
		"--config", "/tmp/client.ovpn",
		"--dev", "tun-test",
		"--dev-type", "tun",
		"--route-nopull",
		"--pull-filter", "ignore", "route-ipv6",
		"--pull-filter", "ignore", "ifconfig-ipv6",
		"--connect-retry-max", "1",
		"--connect-timeout", "15",
		"--auth-nocache",
		"--verb", "3",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("OpenVPNCommand() = %#v, want %#v", got, want)
	}
}

func TestOpenVPNCommandDefaultsBinary(t *testing.T) {
	got := OpenVPNCommand("", "/tmp/client.ovpn", "tun-test")

	if got[0] != "openvpn" {
		t.Fatalf("OpenVPNCommand()[0] = %q, want openvpn", got[0])
	}
}

func TestOpenVPNProcessStarterReceivesCommand(t *testing.T) {
	starter := &recordingProcessStarter{}
	process, err := starter.Start(context.Background(), []string{"openvpn", "--config", "/tmp/client.ovpn"})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if process == nil {
		t.Fatal("expected process")
	}
	if len(starter.commands) != 1 || starter.commands[0][0] != "openvpn" {
		t.Fatalf("commands = %#v, want openvpn command", starter.commands)
	}
}

func TestOpenVPNTunnelStartWritesConfigAndStartsProcess(t *testing.T) {
	starter := &recordingProcessStarter{}
	dir := t.TempDir()
	tun := NewOpenVPN(OpenVPNConfig{
		DataDir: dir,
		Command: "/usr/sbin/openvpn",
		Starter: starter,
	})

	n := node.Node{ID: "jp-1", Region: "jp", OpenVPN: "client\nremote vpn-jp.example.net 1194 udp\n"}
	err := tun.Start(context.Background(), n, Options{Name: "jp-10", DeviceName: "rpg0"})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = tun.Stop(context.Background())
	})

	configPath := filepath.Join(dir, "sessions", "jp-10", "client.ovpn")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}
	if string(raw) != n.OpenVPN {
		t.Fatalf("config file = %q, want node openvpn config", string(raw))
	}
	if len(starter.commands) != 1 {
		t.Fatalf("starter command count = %d, want 1", len(starter.commands))
	}
	wantCommand := OpenVPNCommand("/usr/sbin/openvpn", configPath, "rpg0")
	if !reflect.DeepEqual(starter.commands[0], wantCommand) {
		t.Fatalf("starter command = %#v, want %#v", starter.commands[0], wantCommand)
	}
	status := tun.Status()
	if !status.Ready || status.NodeID != "jp-1" || status.Name != "jp-10" || status.PID != 1234 {
		t.Fatalf("status = %+v, want ready jp-1 pid 1234", status)
	}
}

func TestExecOpenVPNProcessStarterDoesNotBindProcessLifetimeToStartContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	process, err := (ExecOpenVPNProcessStarter{}).Start(ctx, []string{"/bin/sh", "-c", "sleep 5"})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- process.Wait()
	}()
	exited := false
	t.Cleanup(func() {
		if exited {
			return
		}
		_ = process.Kill()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("process did not exit during cleanup")
		}
	})

	cancel()

	select {
	case err := <-done:
		exited = true
		t.Fatalf("process exited after start context cancel: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestOpenVPNTunnelStartRejectsMissingConfig(t *testing.T) {
	tun := NewOpenVPN(OpenVPNConfig{DataDir: t.TempDir(), Starter: &recordingProcessStarter{}})
	err := tun.Start(context.Background(), node.Node{ID: "jp-1", Region: "jp"}, Options{Name: "jp-10"})
	if err == nil {
		t.Fatalf("expected missing OpenVPN config error")
	}
	status := tun.Status()
	if status.Error == "" {
		t.Fatalf("expected status error after missing config")
	}
}

func TestOpenVPNTunnelDialBindsToStartedDevice(t *testing.T) {
	dialer := &recordingDeviceDialer{conn: &dummyConn{}}
	tun := NewOpenVPN(OpenVPNConfig{
		DataDir:      t.TempDir(),
		Starter:      &recordingProcessStarter{},
		DeviceDialer: dialer,
	})
	if err := tun.Start(context.Background(), node.Node{ID: "jp-1", OpenVPN: "client\n"}, Options{Name: "jp-3000", DeviceName: "rpg7"}); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = tun.Stop(context.Background())
	})

	conn, err := tun.DialContext(context.Background(), "tcp", "example.com:443")
	if err != nil {
		t.Fatalf("DialContext returned error: %v", err)
	}
	if conn != dialer.conn {
		t.Fatalf("conn = %#v, want injected conn", conn)
	}
	if dialer.deviceName != "rpg7" {
		t.Fatalf("device = %q, want rpg7", dialer.deviceName)
	}
	if dialer.network != "tcp" || dialer.address != "example.com:443" {
		t.Fatalf("dial target = %s %s, want tcp example.com:443", dialer.network, dialer.address)
	}
}

func TestOpenVPNTunnelDialRequiresStartedTunnel(t *testing.T) {
	tun := NewOpenVPN(OpenVPNConfig{DataDir: t.TempDir(), Starter: &recordingProcessStarter{}})
	_, err := tun.DialContext(context.Background(), "tcp", "example.com:443")
	if err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("DialContext error = %v, want not ready", err)
	}
}

func TestOpenVPNTunnelStopTerminatesProcess(t *testing.T) {
	process := &recordingProcess{}
	starter := &singleProcessStarter{process: process}
	tun := NewOpenVPN(OpenVPNConfig{DataDir: t.TempDir(), Starter: starter})
	err := tun.Start(context.Background(), node.Node{ID: "jp-1", OpenVPN: "client\n"}, Options{Name: "jp-10"})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if err := tun.Stop(context.Background()); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	if !process.terminated.Load() {
		t.Fatalf("expected process to be terminated")
	}
	if !process.waited.Load() {
		t.Fatalf("expected process to be waited")
	}
	if tun.Status().Ready {
		t.Fatalf("expected status not ready after stop")
	}
}

func TestOpenVPNTunnelMonitorsUnexpectedProcessExit(t *testing.T) {
	process := &recordingProcess{waitCh: make(chan error, 1)}
	starter := &singleProcessStarter{process: process}
	tun := NewOpenVPN(OpenVPNConfig{DataDir: t.TempDir(), Starter: starter})
	err := tun.Start(context.Background(), node.Node{ID: "jp-1", OpenVPN: "client\n"}, Options{Name: "jp-10"})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	waitFor(t, func() bool { return process.waitCalls.Load() == 1 }, "monitor to call Wait")
	process.waitCh <- errors.New("openvpn exited with status 1")

	waitFor(t, func() bool {
		status := tun.Status()
		return !status.Ready && status.PID == 0 && strings.Contains(status.Error, "openvpn exited with status 1")
	}, "status to reflect process exit")
}

func TestOpenVPNTunnelStopKillsAndReturnsWhenWaitDoesNotComplete(t *testing.T) {
	waitCh := make(chan error)
	process := &recordingProcess{waitCh: waitCh}
	starter := &singleProcessStarter{process: process}
	tun := NewOpenVPN(OpenVPNConfig{
		DataDir:     t.TempDir(),
		Starter:     starter,
		StopTimeout: 10 * time.Millisecond,
	})
	err := tun.Start(context.Background(), node.Node{ID: "jp-1", OpenVPN: "client\n"}, Options{Name: "jp-10"})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- tun.Stop(context.Background())
	}()

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "timed out waiting for openvpn process to exit after kill") {
			t.Fatalf("Stop error = %v, want bounded wait timeout", err)
		}
	case <-time.After(200 * time.Millisecond):
		close(waitCh)
		t.Fatalf("Stop did not return after timeout and kill")
	}
	if !process.terminated.Load() {
		t.Fatalf("expected process to be terminated")
	}
	if !process.killed.Load() {
		t.Fatalf("expected process to be killed after wait timeout")
	}
	close(waitCh)
}

func TestOpenVPNTunnelSwitchRestartsWithNewNode(t *testing.T) {
	starter := &recordingProcessStarter{}
	dir := t.TempDir()
	tun := NewOpenVPN(OpenVPNConfig{DataDir: dir, Starter: starter})
	first := node.Node{ID: "jp-1", OpenVPN: "client\nremote first 1194 udp\n"}
	second := node.Node{ID: "jp-2", OpenVPN: "client\nremote second 1194 udp\n"}
	if err := tun.Start(context.Background(), first, Options{Name: "jp-10", DeviceName: "rpg0"}); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	firstProcess := starter.processes[0]

	if err := tun.Switch(context.Background(), second); err != nil {
		t.Fatalf("Switch returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = tun.Stop(context.Background())
	})

	if !firstProcess.terminated.Load() || !firstProcess.waited.Load() {
		t.Fatalf("expected first process to be stopped before switch")
	}
	if len(starter.commands) != 2 {
		t.Fatalf("starter command count = %d, want 2", len(starter.commands))
	}
	status := tun.Status()
	if status.NodeID != "jp-2" || !status.Ready {
		t.Fatalf("status = %+v, want ready jp-2", status)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "sessions", "jp-10", "client.ovpn"))
	if err != nil {
		t.Fatalf("read switched config: %v", err)
	}
	if string(raw) != second.OpenVPN {
		t.Fatalf("config file = %q, want second node config", string(raw))
	}
}

type recordingProcessStarter struct {
	commands  [][]string
	processes []*recordingProcess
}

func (s *recordingProcessStarter) Start(ctx context.Context, command []string) (OpenVPNProcess, error) {
	s.commands = append(s.commands, append([]string(nil), command...))
	process := &recordingProcess{}
	s.processes = append(s.processes, process)
	return process, nil
}

type recordingProcess struct {
	terminated atomic.Bool
	killed     atomic.Bool
	waited     atomic.Bool
	waitCalls  atomic.Int32
	waitCh     chan error
}

func (p *recordingProcess) PID() int { return 1234 }
func (p *recordingProcess) Wait() error {
	p.waited.Store(true)
	p.waitCalls.Add(1)
	if p.waitCh != nil {
		return <-p.waitCh
	}
	for {
		if p.terminated.Load() || p.killed.Load() {
			return nil
		}
		time.Sleep(time.Millisecond)
	}
}
func (p *recordingProcess) Terminate() error {
	p.terminated.Store(true)
	return nil
}
func (p *recordingProcess) Kill() error {
	p.killed.Store(true)
	return nil
}

type singleProcessStarter struct {
	process OpenVPNProcess
}

func (s *singleProcessStarter) Start(ctx context.Context, command []string) (OpenVPNProcess, error) {
	return s.process, nil
}

type recordingDeviceDialer struct {
	conn       net.Conn
	deviceName string
	network    string
	address    string
}

func (d *recordingDeviceDialer) DialContext(ctx context.Context, deviceName, network, address string) (net.Conn, error) {
	d.deviceName = deviceName
	d.network = network
	d.address = address
	return d.conn, nil
}

type dummyConn struct{}

func (dummyConn) Read(b []byte) (int, error)         { return 0, errors.New("not implemented") }
func (dummyConn) Write(b []byte) (int, error)        { return 0, errors.New("not implemented") }
func (dummyConn) Close() error                       { return nil }
func (dummyConn) LocalAddr() net.Addr                { return nil }
func (dummyConn) RemoteAddr() net.Addr               { return nil }
func (dummyConn) SetDeadline(t time.Time) error      { return nil }
func (dummyConn) SetReadDeadline(t time.Time) error  { return nil }
func (dummyConn) SetWriteDeadline(t time.Time) error { return nil }

func waitFor(t *testing.T, condition func() bool, description string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}
