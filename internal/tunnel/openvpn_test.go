package tunnel

import (
	"context"
	"reflect"
	"testing"
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

type recordingProcessStarter struct {
	commands [][]string
}

func (s *recordingProcessStarter) Start(ctx context.Context, command []string) (OpenVPNProcess, error) {
	s.commands = append(s.commands, append([]string(nil), command...))
	return &recordingProcess{}, nil
}

type recordingProcess struct {
	terminated bool
	killed     bool
}

func (p *recordingProcess) PID() int    { return 1234 }
func (p *recordingProcess) Wait() error { return nil }
func (p *recordingProcess) Terminate() error {
	p.terminated = true
	return nil
}
func (p *recordingProcess) Kill() error {
	p.killed = true
	return nil
}
