package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

type DeviceWaiter func(ctx context.Context, deviceName string, timeout time.Duration) error

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
	DataDir          string
	Command          string
	Starter          OpenVPNProcessStarter
	DeviceDialer     DeviceDialer
	DeviceWaiter     DeviceWaiter
	ReadinessTimeout time.Duration
	StopTimeout      time.Duration
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

type openVPNSession struct {
	process     OpenVPNProcess
	monitorDone chan error
	options     Options
	configPath  string
	startedNode node.Node
	status      Status
}

func NewOpenVPN(cfg OpenVPNConfig) *OpenVPN {
	useSystemStarter := cfg.Starter == nil
	if cfg.Starter == nil {
		cfg.Starter = ExecOpenVPNProcessStarter{}
	}
	if cfg.DeviceDialer == nil {
		cfg.DeviceDialer = SystemDeviceDialer{}
	}
	if cfg.DeviceWaiter == nil {
		if useSystemStarter {
			cfg.DeviceWaiter = waitForDeviceReady
		} else {
			cfg.DeviceWaiter = assumeDeviceReady
		}
	}
	if cfg.ReadinessTimeout == 0 {
		cfg.ReadinessTimeout = 30 * time.Second
	}
	if cfg.StopTimeout == 0 {
		cfg.StopTimeout = 5 * time.Second
	}
	return &OpenVPN{cfg: cfg}
}

func (o *OpenVPN) Start(ctx context.Context, n node.Node, opts Options) error {
	o.mu.Lock()

	if o.process != nil {
		err := fmt.Errorf("openvpn tunnel %q is already started", o.status.Name)
		o.status.Error = err.Error()
		o.mu.Unlock()
		return err
	}
	if n.OpenVPN == "" {
		err := fmt.Errorf("node %q has empty openvpn config", n.ID)
		o.status.Error = err.Error()
		o.mu.Unlock()
		return err
	}
	if opts.Name == "" {
		opts.Name = n.ID
	}
	if opts.DeviceName == "" {
		opts.DeviceName = "rpg0"
	}

	o.mu.Unlock()

	session, err := o.startSession(ctx, n, opts)
	if err != nil {
		o.setError(err)
		return err
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	o.applySessionLocked(session)
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

	err := o.stopProcess(ctx, process, done)
	o.finishStop()
	return err
}

func (o *OpenVPN) startSession(ctx context.Context, n node.Node, opts Options) (openVPNSession, error) {
	if n.OpenVPN == "" {
		return openVPNSession{}, fmt.Errorf("node %q has empty openvpn config", n.ID)
	}
	if opts.Name == "" {
		opts.Name = n.ID
	}
	if opts.DeviceName == "" {
		opts.DeviceName = "rpg0"
	}

	dataDir := firstNonEmpty(opts.DataDir, o.cfg.DataDir)
	sessionDir := filepath.Join(dataDir, "sessions", opts.Name)
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		return openVPNSession{}, fmt.Errorf("create openvpn session dir: %w", err)
	}
	configPath := filepath.Join(sessionDir, "client.ovpn")
	if err := os.WriteFile(configPath, []byte(n.OpenVPN), 0600); err != nil {
		return openVPNSession{}, fmt.Errorf("write openvpn config: %w", err)
	}

	command := OpenVPNCommand(firstNonEmpty(opts.Command, o.cfg.Command), configPath, opts.DeviceName)
	process, err := o.cfg.Starter.Start(ctx, command)
	if err != nil {
		return openVPNSession{}, fmt.Errorf("start openvpn: %w", err)
	}

	monitorDone := make(chan error, 1)
	go o.monitorProcess(process, monitorDone)
	status := Status{Name: opts.Name, NodeID: n.ID, Ready: false, StartedAt: time.Now(), PID: process.PID()}
	if err := o.waitUntilSessionReadyOrProcessExits(ctx, monitorDone, opts.DeviceName); err != nil {
		_ = o.stopProcess(context.Background(), process, monitorDone)
		return openVPNSession{}, fmt.Errorf("wait for openvpn device %q: %w", opts.DeviceName, err)
	}
	status.Ready = true

	return openVPNSession{
		process:     process,
		monitorDone: monitorDone,
		options:     opts,
		configPath:  configPath,
		startedNode: n,
		status:      status,
	}, nil
}

func (o *OpenVPN) applySessionLocked(session openVPNSession) {
	o.process = session.process
	o.monitorDone = session.monitorDone
	o.stopping = false
	o.options = session.options
	o.configPath = session.configPath
	o.startedNode = session.startedNode
	o.status = session.status
	o.status.Error = ""
}

func (o *OpenVPN) stopProcess(ctx context.Context, process OpenVPNProcess, done <-chan error) error {
	if process == nil {
		return nil
	}
	_ = process.Terminate()

	timer := time.NewTimer(o.cfg.StopTimeout)
	defer timer.Stop()

	var err error
	select {
	case err = <-done:
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
		if err != nil {
			return err
		}
		return waitErr
	case <-killTimer.C:
		if err != nil {
			return err
		}
		return fmt.Errorf("timed out waiting for openvpn process to exit after kill")
	}
}

func (o *OpenVPN) Switch(ctx context.Context, n node.Node) error {
	o.mu.Lock()
	opts := o.options
	oldProcess := o.process
	oldDone := o.monitorDone
	o.mu.Unlock()

	nextOpts := opts
	nextOpts.DeviceName = nextSwitchDeviceName(opts.DeviceName)
	newSession, err := o.startSession(ctx, n, nextOpts)
	if err != nil {
		o.setError(err)
		return err
	}

	o.mu.Lock()
	o.applySessionLocked(newSession)
	o.mu.Unlock()

	if oldProcess != nil {
		_ = o.stopProcess(ctx, oldProcess, oldDone)
	}
	return nil
}

func nextSwitchDeviceName(current string) string {
	if current == "" {
		return "rpg0"
	}
	if strings.HasSuffix(current, "n") {
		return strings.TrimSuffix(current, "n")
	}
	if len(current) >= 15 {
		return current[:14] + "n"
	}
	return current + "n"
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

func (o *OpenVPN) waitUntilDeviceReadyOrProcessExits(ctx context.Context, process OpenVPNProcess, deviceName string) error {
	readyDone := make(chan error, 1)
	go func() {
		readyDone <- o.cfg.DeviceWaiter(ctx, deviceName, o.cfg.ReadinessTimeout)
	}()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case err := <-readyDone:
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			o.mu.RLock()
			currentProcess := o.process
			statusErr := o.status.Error
			o.mu.RUnlock()
			if currentProcess != process {
				if statusErr != "" {
					return errors.New(statusErr)
				}
				return fmt.Errorf("openvpn process exited before device became ready")
			}
		}
	}
}

func (o *OpenVPN) waitUntilSessionReadyOrProcessExits(ctx context.Context, done <-chan error, deviceName string) error {
	readyDone := make(chan error, 1)
	go func() {
		readyDone <- o.cfg.DeviceWaiter(ctx, deviceName, o.cfg.ReadinessTimeout)
	}()

	select {
	case err := <-readyDone:
		return err
	case err := <-done:
		if err != nil {
			return err
		}
		return fmt.Errorf("openvpn process exited before device became ready")
	case <-ctx.Done():
		return ctx.Err()
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

func assumeDeviceReady(ctx context.Context, deviceName string, timeout time.Duration) error {
	return ctx.Err()
}

func waitForDeviceReady(ctx context.Context, deviceName string, timeout time.Duration) error {
	if deviceName == "" {
		return fmt.Errorf("openvpn device name is empty")
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		iface, err := net.InterfaceByName(deviceName)
		if err == nil && iface.Flags&net.FlagUp != 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			if err != nil {
				return err
			}
			return fmt.Errorf("device %q is not up", deviceName)
		case <-ticker.C:
		}
	}
}
