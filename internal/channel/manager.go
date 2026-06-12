package channel

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/config"
	"github.com/iceqi/region-proxy-gateway/internal/node"
	"github.com/iceqi/region-proxy-gateway/internal/tunnel"
)

const (
	SelectionAuto   = config.SelectionAuto
	SelectionManual = config.SelectionManual

	dialRetryCount = 3
)

type TunnelFactory func(name string) tunnel.Tunnel

type Config struct {
	Channels      []config.Channel
	Nodes         *node.Store
	TunnelFactory TunnelFactory
	DataDir       string
	OpenVPNCmd    string
}

type Manager struct {
	mu        sync.RWMutex
	cfg       Config
	channels  map[string]*runtimeChannel
	cancel    context.CancelFunc
	rotatorWG sync.WaitGroup
}

type runtimeChannel struct {
	cfg         config.Channel
	tunnel      tunnel.Tunnel
	currentNode node.Node
	startedAt   time.Time
	err         string
}

type Snapshot struct {
	ID             string        `json:"id"`
	ListenHost     string        `json:"listen_host"`
	ListenPort     int           `json:"listen_port"`
	Region         string        `json:"region"`
	RotateMinutes  int           `json:"rotate_minutes"`
	SelectionMode  string        `json:"selection_mode"`
	ManualNodeID   string        `json:"manual_node_id,omitempty"`
	Enabled        bool          `json:"enabled"`
	CurrentNodeID  string        `json:"current_node_id"`
	CurrentNode    node.Node     `json:"current_node"`
	TunnelStatus   tunnel.Status `json:"tunnel_status"`
	LastError      string        `json:"last_error"`
	StartedAt      time.Time     `json:"started_at"`
	ProxyURLHTTP   string        `json:"proxy_url_http"`
	ProxyURLSOCKS5 string        `json:"proxy_url_socks5"`
}

func NewManager(cfg Config) *Manager {
	return &Manager{
		cfg:      cfg,
		channels: make(map[string]*runtimeChannel),
	}
}

func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	runCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	for index, ch := range m.cfg.Channels {
		if !ch.Enabled {
			m.channels[ch.ID] = &runtimeChannel{cfg: ch}
			continue
		}
		if err := m.startLocked(ctx, index, ch); err != nil {
			m.channels[ch.ID] = &runtimeChannel{cfg: ch, err: err.Error()}
		}
	}
	m.mu.Unlock()

	m.startRotators(runCtx)
	return nil
}

func (m *Manager) Stop(ctx context.Context) error {
	if m.cancel != nil {
		m.cancel()
	}
	m.rotatorWG.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for _, ch := range m.channels {
		if ch.tunnel == nil {
			continue
		}
		if err := ch.tunnel.Stop(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *Manager) RotateNow(ctx context.Context, channelID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rotateLocked(ctx, channelID)
}

func (m *Manager) SwitchToNode(ctx context.Context, channelID, nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ch, ok := m.channels[channelID]
	if !ok {
		return fmt.Errorf("channel %q not found", channelID)
	}
	n, ok := m.findNode(nodeID)
	if !ok {
		return fmt.Errorf("node %q not found", nodeID)
	}
	if n.Region != ch.cfg.Region {
		return fmt.Errorf("node %q is region %q, channel requires %q", nodeID, n.Region, ch.cfg.Region)
	}
	if ch.tunnel == nil {
		return fmt.Errorf("channel %q is not running", channelID)
	}
	if err := ch.tunnel.Switch(ctx, n); err != nil {
		ch.err = err.Error()
		return err
	}
	ch.currentNode = n
	ch.cfg.SelectionMode = SelectionManual
	ch.cfg.ManualNodeID = n.ID
	ch.err = ""
	return nil
}

func (m *Manager) Snapshot(channelID string) (Snapshot, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ch, ok := m.channels[channelID]
	if !ok {
		return Snapshot{}, false
	}
	return snapshotOf(ch), true
}

func (m *Manager) Snapshots() []Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshots := make([]Snapshot, 0, len(m.channels))
	for _, ch := range m.channels {
		snapshots = append(snapshots, snapshotOf(ch))
	}
	return snapshots
}

func (m *Manager) ConfiguredChannels() []config.Channel {
	channels := make([]config.Channel, len(m.cfg.Channels))
	copy(channels, m.cfg.Channels)
	return channels
}

func (m *Manager) DialContext(ctx context.Context, channelID, network, address string) (net.Conn, error) {
	var lastErr error
	for attempt := 0; attempt <= dialRetryCount; attempt++ {
		tun, err := m.tunnelForDial(channelID)
		if err != nil {
			return nil, err
		}
		conn, err := tun.DialContext(ctx, network, address)
		if err == nil {
			m.clearError(channelID)
			return conn, nil
		}
		lastErr = err
	}

	if err := m.RotateNow(ctx, channelID); err != nil {
		m.setChannelError(channelID, fmt.Sprintf("dial failed after %d retries: %v; rotate failed: %v", dialRetryCount, lastErr, err))
		return nil, fmt.Errorf("dial failed after %d retries: %w; rotate failed: %v", dialRetryCount, lastErr, err)
	}

	tun, err := m.tunnelForDial(channelID)
	if err != nil {
		return nil, err
	}
	conn, err := tun.DialContext(ctx, network, address)
	if err != nil {
		m.setChannelError(channelID, fmt.Sprintf("dial failed after %d retries and node rotation: %v", dialRetryCount, err))
		return nil, fmt.Errorf("dial failed after %d retries and node rotation: %w", dialRetryCount, err)
	}
	m.clearError(channelID)
	return conn, nil
}

func (m *Manager) tunnelForDial(channelID string) (tunnel.Tunnel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ch, ok := m.channels[channelID]
	if !ok || ch.tunnel == nil {
		return nil, fmt.Errorf("channel %q is not running", channelID)
	}
	return ch.tunnel, nil
}

func (m *Manager) setChannelError(channelID string, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ch, ok := m.channels[channelID]; ok {
		ch.err = message
	}
}

func (m *Manager) clearError(channelID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ch, ok := m.channels[channelID]; ok {
		ch.err = ""
	}
}

func (m *Manager) startLocked(ctx context.Context, index int, ch config.Channel) error {
	n, err := m.selectNode(ch)
	if err != nil {
		m.channels[ch.ID] = &runtimeChannel{cfg: ch, err: err.Error()}
		return err
	}
	if m.cfg.TunnelFactory == nil {
		return fmt.Errorf("tunnel factory is required")
	}
	tun := m.cfg.TunnelFactory(ch.ID)
	opts := tunnel.Options{
		Name:       ch.ID,
		DataDir:    m.cfg.DataDir,
		Command:    m.cfg.OpenVPNCmd,
		DeviceName: fmt.Sprintf("rpg%d", index),
	}
	if err := tun.Start(ctx, n, opts); err != nil {
		m.channels[ch.ID] = &runtimeChannel{cfg: ch, tunnel: tun, currentNode: n, err: err.Error()}
		return err
	}
	m.channels[ch.ID] = &runtimeChannel{
		cfg:         ch,
		tunnel:      tun,
		currentNode: n,
		startedAt:   time.Now(),
	}
	return nil
}

func (m *Manager) startRotators(ctx context.Context) {
	for _, ch := range m.cfg.Channels {
		if !ch.Enabled || ch.SelectionMode != SelectionAuto || ch.RotateMinutes <= 0 {
			continue
		}
		channelID := ch.ID
		interval := time.Duration(ch.RotateMinutes) * time.Minute
		m.rotatorWG.Add(1)
		go func() {
			defer m.rotatorWG.Done()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					_ = m.RotateNow(ctx, channelID)
				}
			}
		}()
	}
}

func (m *Manager) rotateLocked(ctx context.Context, channelID string) error {
	ch, ok := m.channels[channelID]
	if !ok {
		return fmt.Errorf("channel %q not found", channelID)
	}
	if !ch.cfg.Enabled {
		return fmt.Errorf("channel %q is disabled", channelID)
	}
	if ch.cfg.SelectionMode != SelectionAuto {
		return fmt.Errorf("channel %q is manual mode", channelID)
	}
	if ch.tunnel == nil {
		return fmt.Errorf("channel %q is not running", channelID)
	}
	n, err := m.selectRotationNode(ch.cfg, ch.currentNode.ID)
	if err != nil {
		ch.err = err.Error()
		return err
	}
	if n.ID == ch.currentNode.ID {
		return nil
	}
	if err := ch.tunnel.Switch(ctx, n); err != nil {
		ch.err = err.Error()
		return err
	}
	ch.currentNode = n
	ch.startedAt = time.Now()
	ch.err = ""
	return nil
}

func (m *Manager) selectNode(ch config.Channel) (node.Node, error) {
	if m.cfg.Nodes == nil {
		return node.Node{}, fmt.Errorf("node store is required")
	}
	if ch.SelectionMode == SelectionManual {
		n, ok := m.findNode(ch.ManualNodeID)
		if !ok {
			return node.Node{}, fmt.Errorf("manual node %q not found", ch.ManualNodeID)
		}
		if n.Region != ch.Region {
			return node.Node{}, fmt.Errorf("manual node %q is region %q, channel requires %q", n.ID, n.Region, ch.Region)
		}
		return n, nil
	}
	n, ok := m.cfg.Nodes.BestByRegion(ch.Region, "")
	if !ok {
		return node.Node{}, fmt.Errorf("no available node for region %q", ch.Region)
	}
	return n, nil
}

func (m *Manager) selectRotationNode(ch config.Channel, currentNodeID string) (node.Node, error) {
	if m.cfg.Nodes == nil {
		return node.Node{}, fmt.Errorf("node store is required")
	}
	n, ok := m.cfg.Nodes.BestByRegion(ch.Region, currentNodeID)
	if !ok {
		return node.Node{}, fmt.Errorf("no available node for region %q", ch.Region)
	}
	return n, nil
}

func (m *Manager) findNode(id string) (node.Node, bool) {
	if m.cfg.Nodes == nil {
		return node.Node{}, false
	}
	for _, n := range m.cfg.Nodes.List() {
		if n.ID == id {
			return n, true
		}
	}
	return node.Node{}, false
}

func snapshotOf(ch *runtimeChannel) Snapshot {
	status := tunnel.Status{}
	if ch.tunnel != nil {
		status = ch.tunnel.Status()
	}
	return Snapshot{
		ID:             ch.cfg.ID,
		ListenHost:     ch.cfg.ListenHost,
		ListenPort:     ch.cfg.ListenPort,
		Region:         ch.cfg.Region,
		RotateMinutes:  ch.cfg.RotateMinutes,
		SelectionMode:  ch.cfg.SelectionMode,
		ManualNodeID:   ch.cfg.ManualNodeID,
		Enabled:        ch.cfg.Enabled,
		CurrentNodeID:  ch.currentNode.ID,
		CurrentNode:    ch.currentNode,
		TunnelStatus:   status,
		LastError:      ch.err,
		StartedAt:      ch.startedAt,
		ProxyURLHTTP:   fmt.Sprintf("http://%s:%d", ch.cfg.ListenHost, ch.cfg.ListenPort),
		ProxyURLSOCKS5: fmt.Sprintf("socks5://%s:%d", ch.cfg.ListenHost, ch.cfg.ListenPort),
	}
}
