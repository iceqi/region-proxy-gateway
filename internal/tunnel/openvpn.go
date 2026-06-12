package tunnel

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

type OpenVPNProcess interface {
	PID() int
	Wait() error
	Terminate() error
	Kill() error
}

type OpenVPNProcessStarter interface {
	Start(ctx context.Context, command []string) (OpenVPNProcess, error)
}

type ExecOpenVPNProcessStarter struct{}

func (ExecOpenVPNProcessStarter) Start(ctx context.Context, command []string) (OpenVPNProcess, error) {
	if len(command) == 0 {
		return nil, fmt.Errorf("openvpn command is empty")
	}
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &execOpenVPNProcess{cmd: cmd}, nil
}

type execOpenVPNProcess struct {
	cmd *exec.Cmd
}

func (p *execOpenVPNProcess) PID() int {
	if p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func (p *execOpenVPNProcess) Wait() error {
	return p.cmd.Wait()
}

func (p *execOpenVPNProcess) Terminate() error {
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Signal(os.Interrupt)
}

func (p *execOpenVPNProcess) Kill() error {
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

func OpenVPNCommand(binary string, configPath string, deviceName string) []string {
	if binary == "" {
		binary = "openvpn"
	}
	if deviceName == "" {
		deviceName = "rpg0"
	}

	return []string{
		binary,
		"--config", configPath,
		"--dev", deviceName,
		"--dev-type", "tun",
		"--route-nopull",
		"--pull-filter", "ignore", "route-ipv6",
		"--pull-filter", "ignore", "ifconfig-ipv6",
		"--connect-retry-max", "1",
		"--connect-timeout", "15",
		"--auth-nocache",
		"--verb", "3",
	}
}
