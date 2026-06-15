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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cmd := exec.Command(command[0], command[1:]...)
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
	DataDir      string
	Command      string
	Starter      OpenVPNProcessStarter
	DeviceDialer DeviceDialer
	StopTimeout  time.Duration
}

type OpenVPN struct {
	mu          sync.RWMutex
	cfg         OpenVPNConfig
	status      Status
	process     OpenVPNProcess
	monitorDone chan error
	stopping    bool
	options     Options
	configPath  string
	startedNode node.Node
}

func NewOpenVPN(cfg OpenVPNConfig) *OpenVPN {
	if cfg.Starter == nil {
		cfg.Starter = ExecOpenVPNProcessStarter{}
	}
	if cfg.DeviceDialer == nil {
		cfg.DeviceDialer = SystemDeviceDialer{}
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
	o.monitorDone = make(chan error, 1)
	o.stopping = false
	o.options = opts
	o.configPath = configPath
	o.startedNode = n
	o.status = Status{Name: opts.Name, NodeID: n.ID, Ready: true, StartedAt: time.Now(), PID: process.PID()}
	go o.monitorProcess(process, o.monitorDone)
	return nil
}

func (o *OpenVPN) Stop(ctx context.Context) error {
	o.mu.Lock()
	process := o.process
	done := o.monitorDone
	if process == nil {
		o.status.Ready = false
		o.status.PID = 0
		o.mu.Unlock()
		return nil
	}
	o.status.Ready = false
	o.status.PID = 0
	o.stopping = true
	o.mu.Unlock()

	_ = process.Terminate()

	timer := time.NewTimer(o.cfg.StopTimeout)
	defer timer.Stop()

	var err error
	select {
	case err = <-done:
		o.finishStop()
		return err
	case <-ctx.Done():
		_ = process.Kill()
		err = ctx.Err()
	case <-timer.C:
		_ = process.Kill()
	}

	killTimer := time.NewTimer(100 * time.Millisecond)
	defer killTimer.Stop()

	select {
	case waitErr := <-done:
		o.finishStop()
		if err != nil {
			return err
		}
		return waitErr
	case <-killTimer.C:
		o.finishStop()
		if err != nil {
			return err
		}
		return fmt.Errorf("timed out waiting for openvpn process to exit after kill")
	}
}

func (o *OpenVPN) Switch(ctx context.Context, n node.Node) error {
	o.mu.Lock()
	opts := o.options
	previousNode := o.startedNode
	o.mu.Unlock()

	if err := o.Stop(ctx); err != nil {
		o.setError(err)
		return err
	}
	if err := o.Start(ctx, n, opts); err != nil {
		startErr := err
		if previousNode.OpenVPN != "" {
			if rollbackErr := o.Start(ctx, previousNode, opts); rollbackErr != nil {
				err = fmt.Errorf("%w; rollback to %q failed: %v", startErr, previousNode.ID, rollbackErr)
			}
		}
		o.setError(err)
		return err
	}
	return nil
}

func (o *OpenVPN) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	o.mu.RLock()
	ready := o.status.Ready
	deviceName := o.options.DeviceName
	dialer := o.cfg.DeviceDialer
	o.mu.RUnlock()

	if !ready {
		return nil, fmt.Errorf("openvpn tunnel is not ready")
	}
	if deviceName == "" {
		return nil, fmt.Errorf("openvpn tunnel has no device name")
	}
	conn, err := dialer.DialContext(ctx, deviceName, network, address)
	if err != nil {
		o.markDialFailure(err)
		return nil, err
	}
	return conn, nil
}

func (o *OpenVPN) Status() Status {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.status
}

func (o *OpenVPN) monitorProcess(process OpenVPNProcess, done chan<- error) {
	err := process.Wait()

	o.mu.Lock()
	expected := o.stopping && o.process == process
	if o.process == process {
		o.process = nil
		o.monitorDone = nil
		o.status.Ready = false
		o.status.PID = 0
		if !expected {
			if err != nil {
				o.status.Error = err.Error()
			} else {
				o.status.Error = "openvpn process exited"
			}
		}
	}
	o.mu.Unlock()

	done <- err
}

func (o *OpenVPN) finishStop() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.process = nil
	o.monitorDone = nil
	o.stopping = false
	o.status.Ready = false
	o.status.PID = 0
}

func (o *OpenVPN) setError(err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err != nil {
		o.status.Error = err.Error()
	}
}

func (o *OpenVPN) markDialFailure(err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.status.Ready = false
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
