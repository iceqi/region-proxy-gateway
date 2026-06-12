package tunnel

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/node"
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
	if p.cmd == nil {
		return nil
	}
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

type OpenVPNConfig struct {
	DataDir     string
	Command     string
	Starter     OpenVPNProcessStarter
	StopTimeout time.Duration
}

type OpenVPN struct {
	mu          sync.RWMutex
	cfg         OpenVPNConfig
	status      Status
	process     OpenVPNProcess
	options     Options
	configPath  string
	startedNode node.Node
}

func NewOpenVPN(cfg OpenVPNConfig) *OpenVPN {
	if cfg.Starter == nil {
		cfg.Starter = ExecOpenVPNProcessStarter{}
	}
	if cfg.StopTimeout == 0 {
		cfg.StopTimeout = 5 * time.Second
	}
	return &OpenVPN{cfg: cfg}
}

func (o *OpenVPN) Start(ctx context.Context, n node.Node, opts Options) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.process != nil {
		err := fmt.Errorf("openvpn tunnel %q is already started", o.status.Name)
		o.status.Error = err.Error()
		return err
	}
	if n.OpenVPN == "" {
		err := fmt.Errorf("node %q has empty openvpn config", n.ID)
		o.status.Error = err.Error()
		return err
	}
	if opts.Name == "" {
		opts.Name = n.ID
	}

	dataDir := firstNonEmpty(opts.DataDir, o.cfg.DataDir)
	sessionDir := filepath.Join(dataDir, "sessions", opts.Name)
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		o.status.Error = err.Error()
		return fmt.Errorf("create openvpn session dir: %w", err)
	}
	configPath := filepath.Join(sessionDir, "client.ovpn")
	if err := os.WriteFile(configPath, []byte(n.OpenVPN), 0600); err != nil {
		o.status.Error = err.Error()
		return fmt.Errorf("write openvpn config: %w", err)
	}

	command := OpenVPNCommand(firstNonEmpty(opts.Command, o.cfg.Command), configPath, opts.DeviceName)
	process, err := o.cfg.Starter.Start(ctx, command)
	if err != nil {
		o.status.Error = err.Error()
		return fmt.Errorf("start openvpn: %w", err)
	}

	o.process = process
	o.options = opts
	o.configPath = configPath
	o.startedNode = n
	o.status = Status{Name: opts.Name, NodeID: n.ID, Ready: true, StartedAt: time.Now(), PID: process.PID()}
	return nil
}

func (o *OpenVPN) Stop(ctx context.Context) error {
	o.mu.Lock()
	process := o.process
	if process == nil {
		o.status.Ready = false
		o.status.PID = 0
		o.mu.Unlock()
		return nil
	}
	o.process = nil
	o.status.Ready = false
	o.status.PID = 0
	o.mu.Unlock()

	_ = process.Terminate()
	done := make(chan error, 1)
	go func() {
		done <- process.Wait()
	}()

	timer := time.NewTimer(o.cfg.StopTimeout)
	defer timer.Stop()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = process.Kill()
		return ctx.Err()
	case <-timer.C:
		_ = process.Kill()
		return <-done
	}
}

func (o *OpenVPN) Switch(ctx context.Context, n node.Node) error {
	o.mu.Lock()
	opts := o.options
	o.mu.Unlock()

	if err := o.Stop(ctx); err != nil {
		o.setError(err)
		return err
	}
	if err := o.Start(ctx, n, opts); err != nil {
		o.setError(err)
		return err
	}
	return nil
}

func (o *OpenVPN) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return nil, fmt.Errorf("openvpn dial requires routing isolation / namespace not implemented")
}

func (o *OpenVPN) Status() Status {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.status
}

func (o *OpenVPN) setError(err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err != nil {
		o.status.Error = err.Error()
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
