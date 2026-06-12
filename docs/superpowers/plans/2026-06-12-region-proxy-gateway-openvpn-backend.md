# Region Proxy Gateway OpenVPN Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a first real OpenVPN tunnel backend foundation with local node loading, OpenVPN process lifecycle management, and explicit routing-isolation limits.

**Architecture:** Keep the proxy/session/admin layers unchanged. Add a node file loader, extend config with `tunnel_backend`, add an injectable OpenVPN process runner behind `tunnel.Tunnel`, and wire main to choose fake or OpenVPN backend. OpenVPN `DialContext` must fail clearly until the later routing phase implements network namespaces or policy routing.

**Tech Stack:** Go standard library (`encoding/json`, `os`, `os/exec`, `net`, `context`, `syscall`, `time`), existing internal packages (`config`, `node`, `session`, `tunnel`), shell installer.

---

## File Structure

Create or modify:

```text
region-proxy-gateway/
├── README.md
├── install.sh
├── data/
│   └── nodes.example.json
├── internal/config/config.go
├── internal/config/config_test.go
├── internal/node/loader.go
├── internal/node/loader_test.go
├── internal/tunnel/openvpn.go
├── internal/tunnel/openvpn_test.go
├── cmd/region-proxy-gateway/main.go
└── cmd/region-proxy-gateway/main_test.go
```

Responsibilities:

- `internal/config`: validates backend choice and keeps defaults runnable without root.
- `internal/node/loader.go`: loads and validates local node JSON files.
- `internal/tunnel/openvpn.go`: owns OpenVPN process lifecycle and config materialization.
- `cmd/region-proxy-gateway/main.go`: wires node source and tunnel factory based on config.
- `README.md`, `install.sh`, `data/nodes.example.json`: operator-facing setup and limitation docs.

## Task 1: Add Tunnel Backend Config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write failing config tests**

Add these tests to `internal/config/config_test.go`:

```go
func TestValidateAcceptsKnownTunnelBackends(t *testing.T) {
	for _, backend := range []string{"fake", "openvpn"} {
		t.Run(backend, func(t *testing.T) {
			cfg := Default()
			cfg.TunnelBackend = backend
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate returned error for %q: %v", backend, err)
			}
		})
	}
}

func TestValidateRejectsUnknownTunnelBackend(t *testing.T) {
	cfg := Default()
	cfg.TunnelBackend = "wireguard"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected unknown tunnel backend error")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/config
```

Expected: build fails because `Config.TunnelBackend` does not exist.

- [ ] **Step 3: Implement config field and validation**

Modify `internal/config/config.go`:

```go
const (
	TunnelBackendFake    = "fake"
	TunnelBackendOpenVPN = "openvpn"
)
```

Add to `Config`:

```go
TunnelBackend string `json:"tunnel_backend"`
```

Add to `Default()`:

```go
TunnelBackend: TunnelBackendFake,
```

Add to `Validate()`:

```go
switch c.TunnelBackend {
case TunnelBackendFake, TunnelBackendOpenVPN:
default:
	return fmt.Errorf("tunnel backend must be one of: fake, openvpn")
}
```

- [ ] **Step 4: Verify config tests pass**

Run:

```bash
go test ./internal/config
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: configure tunnel backend"
```

## Task 2: Add Local Node Loader

**Files:**
- Create: `internal/node/loader.go`
- Create: `internal/node/loader_test.go`

- [ ] **Step 1: Write failing loader tests**

Create `internal/node/loader_test.go`:

```go
package node

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFileReadsNodes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodes.json")
	raw := `[
		{
			"id": "jp-1",
			"region": "jp",
			"country": "Japan",
			"ip": "203.0.113.10",
			"hostname": "vpn-jp.example.net",
			"openvpn": "client\nremote vpn-jp.example.net 1194 udp\n"
		}
	]`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatalf("write nodes file: %v", err)
	}

	nodes, err := LoadFile(path, RequireOpenVPNConfig)
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("node count = %d, want 1", len(nodes))
	}
	if nodes[0].ID != "jp-1" || nodes[0].Region != "jp" || !nodes[0].Available {
		t.Fatalf("node = %+v, want loaded available jp node", nodes[0])
	}
}

func TestLoadFileRejectsDuplicateIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodes.json")
	raw := `[
		{"id":"jp-1","region":"jp","openvpn":"client\n"},
		{"id":"jp-1","region":"jp","openvpn":"client\n"}
	]`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatalf("write nodes file: %v", err)
	}

	if _, err := LoadFile(path, RequireOpenVPNConfig); err == nil {
		t.Fatalf("expected duplicate ID error")
	}
}

func TestLoadFileRejectsMissingOpenVPNWhenRequired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodes.json")
	raw := `[{"id":"jp-1","region":"jp"}]`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatalf("write nodes file: %v", err)
	}

	if _, err := LoadFile(path, RequireOpenVPNConfig); err == nil {
		t.Fatalf("expected missing OpenVPN config error")
	}
}

func TestLoadFileAllowsMissingOpenVPNWhenNotRequired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodes.json")
	raw := `[{"id":"jp-1","region":"jp"}]`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatalf("write nodes file: %v", err)
	}

	nodes, err := LoadFile(path, AllowMissingOpenVPNConfig)
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("node count = %d, want 1", len(nodes))
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/node
```

Expected: build fails because `LoadFile`, `RequireOpenVPNConfig`, and `AllowMissingOpenVPNConfig` do not exist.

- [ ] **Step 3: Implement loader**

Create `internal/node/loader.go`:

```go
package node

import (
	"encoding/json"
	"fmt"
	"os"
)

type OpenVPNRequirement bool

const (
	AllowMissingOpenVPNConfig OpenVPNRequirement = false
	RequireOpenVPNConfig     OpenVPNRequirement = true
)

func LoadFile(path string, requireOpenVPN OpenVPNRequirement) ([]Node, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read nodes file %q: %w", path, err)
	}

	var nodes []Node
	if err := json.Unmarshal(raw, &nodes); err != nil {
		return nil, fmt.Errorf("parse nodes file %q: %w", path, err)
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("nodes file %q contains no nodes", path)
	}

	seen := map[string]struct{}{}
	for i := range nodes {
		if nodes[i].ID == "" {
			return nil, fmt.Errorf("node at index %d has empty id", i)
		}
		if nodes[i].Region == "" {
			return nil, fmt.Errorf("node %q has empty region", nodes[i].ID)
		}
		if requireOpenVPN && nodes[i].OpenVPN == "" {
			return nil, fmt.Errorf("node %q has empty openvpn config", nodes[i].ID)
		}
		if _, ok := seen[nodes[i].ID]; ok {
			return nil, fmt.Errorf("duplicate node id %q", nodes[i].ID)
		}
		seen[nodes[i].ID] = struct{}{}
		nodes[i].Available = true
	}

	return nodes, nil
}
```

- [ ] **Step 4: Verify node tests pass**

Run:

```bash
go test ./internal/node
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/node/loader.go internal/node/loader_test.go
git commit -m "feat: load local node file"
```

## Task 3: Add Injectable Process Runner For OpenVPN

**Files:**
- Modify: `internal/tunnel/openvpn.go`
- Modify: `internal/tunnel/openvpn_test.go`

- [ ] **Step 1: Write failing process-runner tests**

Append to `internal/tunnel/openvpn_test.go`:

```go
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

func (p *recordingProcess) PID() int { return 1234 }
func (p *recordingProcess) Wait() error { return nil }
func (p *recordingProcess) Terminate() error {
	p.terminated = true
	return nil
}
func (p *recordingProcess) Kill() error {
	p.killed = true
	return nil
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/tunnel
```

Expected: build fails because `OpenVPNProcess` does not exist.

- [ ] **Step 3: Implement process interfaces and exec starter**

Replace `internal/tunnel/openvpn.go` with command builder plus process types:

```go
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
```

- [ ] **Step 4: Verify tunnel tests pass**

Run:

```bash
go test ./internal/tunnel
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tunnel/openvpn.go internal/tunnel/openvpn_test.go
git commit -m "feat: add openvpn process runner"
```

## Task 4: Implement OpenVPN Tunnel Lifecycle

**Files:**
- Modify: `internal/tunnel/openvpn.go`
- Modify: `internal/tunnel/openvpn_test.go`

- [ ] **Step 1: Write failing lifecycle tests**

Append to `internal/tunnel/openvpn_test.go`:

```go
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
	status := tun.Status()
	if !status.Ready || status.NodeID != "jp-1" || status.Name != "jp-10" || status.PID != 1234 {
		t.Fatalf("status = %+v, want ready jp-1 pid 1234", status)
	}
}

func TestOpenVPNTunnelStartRejectsMissingConfig(t *testing.T) {
	tun := NewOpenVPN(OpenVPNConfig{DataDir: t.TempDir(), Starter: &recordingProcessStarter{}})
	err := tun.Start(context.Background(), node.Node{ID: "jp-1", Region: "jp"}, Options{Name: "jp-10"})
	if err == nil {
		t.Fatalf("expected missing OpenVPN config error")
	}
}

func TestOpenVPNTunnelDialReturnsRoutingError(t *testing.T) {
	tun := NewOpenVPN(OpenVPNConfig{DataDir: t.TempDir(), Starter: &recordingProcessStarter{}})
	_, err := tun.DialContext(context.Background(), "tcp", "example.com:443")
	if err == nil {
		t.Fatalf("expected routing isolation error")
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
	if !process.terminated {
		t.Fatalf("expected process to be terminated")
	}
	if tun.Status().Ready {
		t.Fatalf("expected status not ready after stop")
	}
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

	if err := tun.Switch(context.Background(), second); err != nil {
		t.Fatalf("Switch returned error: %v", err)
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

type singleProcessStarter struct {
	process OpenVPNProcess
}

func (s *singleProcessStarter) Start(ctx context.Context, command []string) (OpenVPNProcess, error) {
	return s.process, nil
}
```

Also update imports in `internal/tunnel/openvpn_test.go` to include:

```go
import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/iceqi/region-proxy-gateway/internal/node"
)
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/tunnel
```

Expected: build fails because `NewOpenVPN`, `OpenVPNConfig`, and `Status.PID` do not exist.

- [ ] **Step 3: Extend tunnel status**

Modify `internal/tunnel/tunnel.go`:

```go
type Status struct {
	Name      string    `json:"name"`
	NodeID    string    `json:"node_id"`
	Ready     bool      `json:"ready"`
	StartedAt time.Time `json:"started_at"`
	Error     string    `json:"error"`
	PID       int       `json:"pid,omitempty"`
}
```

- [ ] **Step 4: Implement OpenVPN tunnel**

Add to `internal/tunnel/openvpn.go` below `OpenVPNCommand`:

```go
type OpenVPNConfig struct {
	DataDir     string
	Command     string
	Starter     OpenVPNProcessStarter
	StopTimeout time.Duration
}

type OpenVPN struct {
	mu          sync.Mutex
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
		return fmt.Errorf("openvpn tunnel %q is already started", o.status.Name)
	}
	if n.OpenVPN == "" {
		err := fmt.Errorf("node %q has empty openvpn config", n.ID)
		o.status.Error = err.Error()
		return err
	}
	if opts.Name == "" {
		opts.Name = n.ID
	}

	dataDir := opts.DataDir
	if dataDir == "" {
		dataDir = o.cfg.DataDir
	}
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

	now := time.Now()
	o.process = process
	o.options = opts
	o.configPath = configPath
	o.startedNode = n
	o.status = Status{Name: opts.Name, NodeID: n.ID, Ready: true, StartedAt: now, PID: process.PID()}
	return nil
}

func (o *OpenVPN) Stop(ctx context.Context) error {
	o.mu.Lock()
	process := o.process
	if process == nil {
		o.status.Ready = false
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

	timeout := o.cfg.StopTimeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	timer := time.NewTimer(timeout)
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
	return nil, fmt.Errorf("openvpn dial requires routing isolation; network namespace backend is not implemented")
}

func (o *OpenVPN) Status() Status {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.status
}

func (o *OpenVPN) setError(err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.status.Ready = false
	o.status.Error = err.Error()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
```

Update imports in `internal/tunnel/openvpn.go`:

```go
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
```

- [ ] **Step 5: Verify tunnel tests pass**

Run:

```bash
go test ./internal/tunnel
```

Expected: PASS.

- [ ] **Step 6: Verify all tests pass**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/tunnel/openvpn.go internal/tunnel/openvpn_test.go internal/tunnel/tunnel.go
git commit -m "feat: implement openvpn tunnel lifecycle"
```

## Task 5: Wire Main To Backend And Node Source

**Files:**
- Modify: `cmd/region-proxy-gateway/main.go`
- Modify: `cmd/region-proxy-gateway/main_test.go`

- [ ] **Step 1: Write failing main wiring tests**

Append to `cmd/region-proxy-gateway/main_test.go`:

```go
func TestBuildServicesLoadsOpenVPNNodesFromDataDir(t *testing.T) {
	dir := t.TempDir()
	raw := `[{"id":"jp-real","region":"jp","openvpn":"client\nremote vpn-jp.example.net 1194 udp\n"}]`
	if err := os.WriteFile(filepath.Join(dir, "nodes.json"), []byte(raw), 0600); err != nil {
		t.Fatalf("write nodes file: %v", err)
	}

	cfg := config.Default()
	cfg.TunnelBackend = config.TunnelBackendOpenVPN
	cfg.DataDir = dir

	services, err := buildServices(cfg)
	if err != nil {
		t.Fatalf("buildServices returned error: %v", err)
	}
	nodes := services.nodes.List()
	if len(nodes) != 1 || nodes[0].ID != "jp-real" {
		t.Fatalf("nodes = %+v, want jp-real from data dir", nodes)
	}
}

func TestBuildServicesOpenVPNRequiresNodeFile(t *testing.T) {
	cfg := config.Default()
	cfg.TunnelBackend = config.TunnelBackendOpenVPN
	cfg.DataDir = t.TempDir()

	if _, err := buildServices(cfg); err == nil {
		t.Fatalf("expected missing nodes file error")
	}
}
```

Update existing tests because `buildServices` will return `(services, error)`:

```go
services, err := buildServices(cfg)
if err != nil {
	t.Fatalf("buildServices returned error: %v", err)
}
```

Update imports to include:

```go
import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./cmd/region-proxy-gateway
```

Expected: build fails because `buildServices` does not return an error and does not load nodes.

- [ ] **Step 3: Change main wiring**

Modify `cmd/region-proxy-gateway/main.go`:

```go
services, err := buildServices(cfg)
if err != nil {
	log.Fatalf("build services: %v", err)
}
```

Change signature:

```go
func buildServices(cfg config.Config) (services, error)
```

Replace hard-coded node setup and factory with:

```go
nodes := node.NewStore()
factory, loadedNodes, err := buildTunnelFactory(cfg)
if err != nil {
	return services{}, err
}
nodes.Replace(loadedNodes)
```

Add helpers:

```go
func buildTunnelFactory(cfg config.Config) (session.Factory, []node.Node, error) {
	switch cfg.TunnelBackend {
	case config.TunnelBackendFake:
		return func(key string) tunnel.Tunnel {
			return tunnel.NewFake(key)
		}, demoNodes(), nil
	case config.TunnelBackendOpenVPN:
		nodes, err := node.LoadFile(filepath.Join(cfg.DataDir, "nodes.json"), node.RequireOpenVPNConfig)
		if err != nil {
			return nil, nil, err
		}
		return func(key string) tunnel.Tunnel {
			return tunnel.NewOpenVPN(tunnel.OpenVPNConfig{
				DataDir: cfg.DataDir,
				Command: cfg.OpenVPNCommand,
			})
		}, nodes, nil
	default:
		return nil, nil, fmt.Errorf("unsupported tunnel backend %q", cfg.TunnelBackend)
	}
}

func demoNodes() []node.Node {
	return []node.Node{
		{ID: "jp-demo", Region: "jp", IP: "203.0.113.10", LatencyMS: 50, Available: true},
		{ID: "us-demo", Region: "us", IP: "198.51.100.10", LatencyMS: 60, Available: true},
	}
}
```

Update imports to add `path/filepath`.

- [ ] **Step 4: Verify command tests pass**

Run:

```bash
go test ./cmd/region-proxy-gateway
```

Expected: PASS.

- [ ] **Step 5: Verify all tests pass**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/region-proxy-gateway/main.go cmd/region-proxy-gateway/main_test.go
git commit -m "feat: wire openvpn backend in main"
```

## Task 6: Update Docs, Example Nodes, And Installer

**Files:**
- Modify: `README.md`
- Modify: `install.sh`
- Create: `data/nodes.example.json`

- [ ] **Step 1: Add example nodes file**

Create `data/nodes.example.json`:

```json
[
  {
    "id": "jp-example-1",
    "region": "jp",
    "country": "Japan",
    "ip": "203.0.113.10",
    "hostname": "vpn-jp.example.net",
    "openvpn": "client\nremote vpn-jp.example.net 1194 udp\nroute-nopull\n"
  }
]
```

- [ ] **Step 2: Update README**

Replace `README.md` with:

~~~markdown
# Region Proxy Gateway

Single-port region proxy gateway written in Go.

Proxy username format:

```text
<region>-<minutes>
```

Examples:

```text
jp-10
us-0
```

`minutes=0` means fixed current node. `minutes>0` means rotate within the same region.

## Current Status

Implemented:

- Single TCP proxy port.
- HTTP proxy and HTTP CONNECT.
- SOCKS5 username/password proxy.
- Username strategy parsing, for example `jp-10`.
- Shared strategy sessions.
- Online connection tracking.
- Admin JSON API.
- Fake tunnel backend.
- OpenVPN process lifecycle backend.

Important limitation:

The OpenVPN backend can start and switch OpenVPN processes, but per-session route isolation is not implemented yet. Until the routing phase adds Linux network namespaces or policy routing, proxy traffic is not silently treated as regional OpenVPN traffic. OpenVPN `DialContext` returns a clear routing-isolation error.

## Proxy Examples

```text
http://jp-10:PASSWORD@SERVER_IP:3000
socks5://jp-10:PASSWORD@SERVER_IP:3000
http://us-0:PASSWORD@SERVER_IP:3000
socks5://us-0:PASSWORD@SERVER_IP:3000
```

## Tunnel Backends

Default backend:

```text
fake
```

The fake backend keeps the service runnable without root privileges or OpenVPN.

OpenVPN backend:

```json
{
  "tunnel_backend": "openvpn",
  "data_dir": "./data",
  "openvpn_command": "openvpn"
}
```

OpenVPN nodes are loaded from:

```text
data/nodes.json
```

Example format:

```json
[
  {
    "id": "jp-example-1",
    "region": "jp",
    "country": "Japan",
    "ip": "203.0.113.10",
    "hostname": "vpn-jp.example.net",
    "openvpn": "client\nremote vpn-jp.example.net 1194 udp\nroute-nopull\n"
  }
]
```

## Admin API

Default admin address:

```text
http://127.0.0.1:8787
```

Endpoints:

```text
GET /api/status
GET /api/sessions
GET /api/nodes
GET /api/connections
```

## Build

```bash
go test ./...
go build -o region-proxy-gateway ./cmd/region-proxy-gateway
```
~~~

- [ ] **Step 3: Update installer**

Modify `install.sh` after `mkdir -p "${INSTALL_DIR}"`:

```bash
mkdir -p "${INSTALL_DIR}/data"
```

After build, add:

```bash
if [[ -f "${INSTALL_DIR}/data/nodes.example.json" && ! -f "${INSTALL_DIR}/data/nodes.json" ]]; then
  cp "${INSTALL_DIR}/data/nodes.example.json" "${INSTALL_DIR}/data/nodes.json.example"
fi
```

Change final echo lines:

```bash
echo "Admin: http://127.0.0.1:8787"
echo "HTTP proxy example: http://jp-10:PASSWORD@SERVER_IP:3000"
echo "SOCKS5 proxy example: socks5://jp-10:PASSWORD@SERVER_IP:3000"
echo "OpenVPN nodes example: ${INSTALL_DIR}/data/nodes.example.json"
```

- [ ] **Step 4: Verify docs and build**

Run:

```bash
go test ./...
go build -o /tmp/region-proxy-gateway ./cmd/region-proxy-gateway
rm -f /tmp/region-proxy-gateway
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add README.md install.sh data/nodes.example.json
git commit -m "docs: document openvpn backend setup"
```

## Task 7: Final Verification And Push

**Files:**
- No code changes expected.

- [ ] **Step 1: Run full test suite**

```bash
go test ./...
```

Expected: every package reports `ok`.

- [ ] **Step 2: Run build**

```bash
go build -o /tmp/region-proxy-gateway ./cmd/region-proxy-gateway
rm -f /tmp/region-proxy-gateway
```

Expected: build succeeds and temporary binary is removed.

- [ ] **Step 3: Check git status**

```bash
git status --short --branch
```

Expected: branch is ahead of `origin/main`; no unstaged or uncommitted files.

- [ ] **Step 4: Push**

```bash
git push
```

Expected: push succeeds to `git@github.com:iceqi/region-proxy-gateway.git`.

- [ ] **Step 5: Report current limitation**

Final user report must explicitly state:

```text
OpenVPN process lifecycle is implemented, but real regional traffic routing still requires the next network namespace/policy-routing phase.
```

This prevents overstating the current backend as a complete regional egress implementation.
