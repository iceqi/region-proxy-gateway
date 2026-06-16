package channel

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/config"
	"github.com/iceqi/region-proxy-gateway/internal/deeptest"
	"github.com/iceqi/region-proxy-gateway/internal/node"
	"github.com/iceqi/region-proxy-gateway/internal/tunnel"
)

const (
	SelectionAuto   = config.SelectionAuto
	SelectionManual = config.SelectionManual

	dialRetryCount = 3
)

type TunnelFactory func(name string) tunnel.Tunnel
type NodeChecker func(context.Context, node.Node) node.Node
type NodeRefresher func(context.Context) error

type NodeUse struct {
	ChannelID   string
	NodeID      string
	ExitIP      string
	ConnectedAt time.Time
	SwitchedAt  time.Time
}

type History interface {
	RecentNodeIDsForChannel(ctx context.Context, channelID string, since time.Time) (map[string]time.Time, error)
	DeepTestResults(ctx context.Context) (map[string]deeptest.Result, error)
	RecordChannelNodeUse(ctx context.Context, use NodeUse) error
}

type Config struct {
	Channels      []config.Channel
	Nodes         *node.Store
	TunnelFactory TunnelFactory
	NodeChecker   NodeChecker
	RefreshNodes  NodeRefresher
	History       History
	DataDir       string
	OpenVPNCmd    string
}

type Manager struct {
	mu               sync.RWMutex
	cfg              Config
	channels         map[string]*runtimeChannel
	lifecycleCtx     context.Context
	lifecycleCancel  context.CancelFunc
	rotatorCancel    context.CancelFunc
	rotatorWG        sync.WaitGroup
	rotationInterval func(config.Channel) time.Duration
}

type runtimeChannel struct {
	cfg            config.Channel
	tunnel         tunnel.Tunnel
	currentNode    node.Node
	lastNode       node.Node
	startedAt      time.Time
	lastRotationAt time.Time
	err            string
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
	LastExitIP     string        `json:"last_exit_ip"`
	CurrentExitIP  string        `json:"current_exit_ip"`
	LastRotationAt time.Time     `json:"last_rotation_at"`
	NextRotationAt time.Time     `json:"next_rotation_at"`
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
	lifecycleCtx, lifecycleCancel := context.WithCancel(ctx)
	m.lifecycleCtx = lifecycleCtx
	m.lifecycleCancel = lifecycleCancel
	runCtx, rotatorCancel := context.WithCancel(lifecycleCtx)
	m.rotatorCancel = rotatorCancel

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
	if m.lifecycleCancel != nil {
		m.lifecycleCancel()
	}
	if m.rotatorCancel != nil {
		m.rotatorCancel()
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

func (m *Manager) ReplaceChannels(ctx context.Context, channels []config.Channel) error {
	if m.rotatorCancel != nil {
		m.rotatorCancel()
	}
	m.rotatorWG.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()

	nextByID := make(map[string]config.Channel, len(channels))
	for _, ch := range channels {
		nextByID[ch.ID] = ch
	}

	for id, runtime := range m.channels {
		if _, ok := nextByID[id]; ok {
			continue
		}
		if runtime.tunnel != nil {
			_ = runtime.tunnel.Stop(ctx)
		}
		delete(m.channels, id)
	}

	m.cfg.Channels = append([]config.Channel(nil), channels...)
	for index, ch := range m.cfg.Channels {
		current, exists := m.channels[ch.ID]
		if !ch.Enabled {
			if exists && current.tunnel != nil {
				_ = current.tunnel.Stop(ctx)
			}
			m.channels[ch.ID] = &runtimeChannel{cfg: ch}
			continue
		}
		if !exists || current.tunnel == nil || channelRuntimeNeedsRestart(current.cfg, ch) {
			if exists && current.tunnel != nil {
				_ = current.tunnel.Stop(ctx)
			}
			if err := m.startLocked(ctx, index, ch); err != nil {
				m.channels[ch.ID] = &runtimeChannel{cfg: ch, err: err.Error()}
			}
			continue
		}
		current.cfg = ch
	}

	baseCtx := m.lifecycleCtx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	runCtx, rotatorCancel := context.WithCancel(baseCtx)
	m.rotatorCancel = rotatorCancel
	m.startRotators(runCtx)
	return nil
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
	if !isAnyRegion(ch.cfg.Region) && n.Region != ch.cfg.Region {
		return fmt.Errorf("node %q is region %q, channel requires %q", nodeID, n.Region, ch.cfg.Region)
	}
	if ch.tunnel == nil {
		if err := m.startChannelWithNodeLocked(ctx, ch, n); err != nil {
			ch.err = err.Error()
			return err
		}
		return nil
	}
	if err := ch.tunnel.Switch(ctx, n); err != nil {
		ch.err = err.Error()
		return err
	}
	ch.lastNode = ch.currentNode
	ch.currentNode = n
	ch.startedAt = time.Now()
	ch.lastRotationAt = ch.startedAt
	ch.cfg.SelectionMode = SelectionManual
	ch.cfg.ManualNodeID = n.ID
	ch.err = ""
	m.recordUse(ctx, ch.cfg.ID, n, ch.startedAt, ch.startedAt)
	return nil
}

func (m *Manager) startChannelWithNodeLocked(ctx context.Context, ch *runtimeChannel, n node.Node) error {
	if m.cfg.TunnelFactory == nil {
		return fmt.Errorf("tunnel factory is required")
	}
	tun := m.cfg.TunnelFactory(ch.cfg.ID)
	opts := tunnel.Options{
		Name:       ch.cfg.ID,
		DataDir:    m.cfg.DataDir,
		Command:    m.cfg.OpenVPNCmd,
		DeviceName: m.deviceNameForChannelLocked(ch.cfg.ID),
	}
	if err := tun.Start(ctx, n, opts); err != nil {
		ch.tunnel = tun
		ch.currentNode = n
		ch.err = err.Error()
		return err
	}
	ch.tunnel = tun
	ch.lastNode = ch.currentNode
	ch.currentNode = n
	ch.startedAt = time.Now()
	ch.lastRotationAt = ch.startedAt
	ch.cfg.Enabled = true
	ch.cfg.SelectionMode = SelectionManual
	ch.cfg.ManualNodeID = n.ID
	ch.err = ""
	m.recordUse(ctx, ch.cfg.ID, n, ch.startedAt, ch.startedAt)
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
	m.mu.RLock()
	defer m.mu.RUnlock()
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

	if conn, err := m.recoverChannelAfterDialFailure(ctx, channelID, network, address); err == nil {
		m.clearError(channelID)
		return conn, nil
	} else {
		m.setChannelError(channelID, fmt.Sprintf("dial failed after %d retries: %v; recovery failed: %v", dialRetryCount, lastErr, err))
		return nil, fmt.Errorf("dial failed after %d retries: %w; recovery failed: %v", dialRetryCount, lastErr, err)
	}
}

func (m *Manager) recoverChannelAfterDialFailure(ctx context.Context, channelID, network, address string) (net.Conn, error) {
	m.mu.RLock()
	ch, ok := m.channels[channelID]
	if !ok {
		m.mu.RUnlock()
		return nil, fmt.Errorf("channel %q not found", channelID)
	}
	selectionMode := ch.cfg.SelectionMode
	m.mu.RUnlock()

	switch selectionMode {
	case SelectionAuto:
		m.mu.RLock()
		ch := m.channels[channelID]
		rotateMinutes := 0
		if ch != nil {
			rotateMinutes = ch.cfg.RotateMinutes
		}
		m.mu.RUnlock()
		if rotateMinutes <= 0 {
			return m.recoverCurrentChannelNode(ctx, channelID, network, address)
		}
		return m.recoverAutoChannel(ctx, channelID, network, address)
	case SelectionManual:
		return m.recoverCurrentChannelNode(ctx, channelID, network, address)
	default:
		return nil, fmt.Errorf("channel %q has unsupported selection mode %q", channelID, selectionMode)
	}
}

func (m *Manager) recoverAutoChannel(ctx context.Context, channelID, network, address string) (net.Conn, error) {
	var lastErr error
	tried := map[string]struct{}{}
	for {
		m.mu.Lock()
		ch, ok := m.channels[channelID]
		if !ok {
			m.mu.Unlock()
			return nil, fmt.Errorf("channel %q not found", channelID)
		}
		if !ch.cfg.Enabled {
			m.mu.Unlock()
			return nil, fmt.Errorf("channel %q is disabled", channelID)
		}
		if ch.tunnel == nil {
			m.mu.Unlock()
			return nil, fmt.Errorf("channel %q is not running", channelID)
		}
		currentNode := ch.currentNode
		if currentNode.ID != "" {
			tried[currentNode.ID] = struct{}{}
		}
		occupiedExitIPs := m.currentExitIPsLocked()
		n, err := m.selectRotationNodeAvoidingLocked(ctx, ch.cfg, currentNode.ID, tried, occupiedExitIPs)
		if err != nil {
			m.mu.Unlock()
			if lastErr != nil {
				return nil, fmt.Errorf("%w; no more candidates: %v", lastErr, err)
			}
			return nil, err
		}
		tried[n.ID] = struct{}{}
		if err := ch.tunnel.Switch(ctx, n); err != nil {
			ch.err = err.Error()
			lastErr = err
			m.mu.Unlock()
			continue
		}
		tun := ch.tunnel
		m.mu.Unlock()

		conn, err := tun.DialContext(ctx, network, address)
		if err == nil {
			m.commitRecoveredNode(ctx, channelID, currentNode, n)
			return conn, nil
		}
		lastErr = err
		m.setChannelError(channelID, fmt.Sprintf("recovered node %q failed dial: %v", n.ID, err))
	}
}

func (m *Manager) recoverCurrentChannelNode(ctx context.Context, channelID, network, address string) (net.Conn, error) {
	m.mu.Lock()
	ch, ok := m.channels[channelID]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("channel %q not found", channelID)
	}
	if ch.tunnel == nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("channel %q is not running", channelID)
	}
	currentNode := ch.currentNode
	if currentNode.ID == "" {
		m.mu.Unlock()
		return nil, fmt.Errorf("channel %q has no current node", channelID)
	}
	if err := ch.tunnel.Switch(ctx, currentNode); err != nil {
		ch.err = err.Error()
		m.mu.Unlock()
		return nil, err
	}
	tun := ch.tunnel
	m.mu.Unlock()

	conn, err := tun.DialContext(ctx, network, address)
	if err != nil {
		m.setChannelError(channelID, err.Error())
		return nil, err
	}
	m.commitRecoveredNode(ctx, channelID, currentNode, currentNode)
	return conn, nil
}

func (m *Manager) commitRecoveredNode(ctx context.Context, channelID string, previous node.Node, current node.Node) {
	m.mu.Lock()
	ch, ok := m.channels[channelID]
	if !ok {
		m.mu.Unlock()
		return
	}
	now := time.Now()
	if previous.ID != current.ID {
		ch.lastNode = previous
		ch.lastRotationAt = now
	}
	ch.currentNode = current
	ch.startedAt = now
	ch.err = ""
	channelID = ch.cfg.ID
	m.mu.Unlock()

	m.recordUse(ctx, channelID, current, now, now)
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
	n, err := m.selectNode(ctx, ch)
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
		DeviceName: m.deviceNameForIndex(index),
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
	m.recordUse(ctx, ch.ID, n, m.channels[ch.ID].startedAt, m.channels[ch.ID].startedAt)
	return nil
}

func (m *Manager) deviceNameForChannelLocked(channelID string) string {
	for i, ch := range m.cfg.Channels {
		if ch.ID == channelID {
			return m.deviceNameForIndex(i)
		}
	}
	return m.deviceNameForIndex(len(m.channels))
}

func (m *Manager) deviceNameForIndex(index int) string {
	if index < 0 {
		index = 0
	}
	return fmt.Sprintf("rpg%d", index)
}

func channelRuntimeNeedsRestart(old config.Channel, next config.Channel) bool {
	return old.ListenHost != next.ListenHost ||
		old.ListenPort != next.ListenPort ||
		old.Region != next.Region ||
		old.SelectionMode != next.SelectionMode ||
		old.ManualNodeID != next.ManualNodeID
}

func (m *Manager) startRotators(ctx context.Context) {
	for _, ch := range m.cfg.Channels {
		if !ch.Enabled || ch.SelectionMode != SelectionAuto || ch.RotateMinutes <= 0 {
			continue
		}
		channelID := ch.ID
		interval := time.Duration(ch.RotateMinutes) * time.Minute
		if m.rotationInterval != nil {
			interval = m.rotationInterval(ch)
		}
		if interval <= 0 {
			continue
		}
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
	if m.cfg.RefreshNodes != nil {
		if err := m.cfg.RefreshNodes(ctx); err != nil {
			ch.err = fmt.Sprintf("refresh nodes before rotation failed: %v", err)
		}
	}
	return m.rotateThroughCandidatesLocked(ctx, channelID, ch)
}

func (m *Manager) rotateThroughCandidatesLocked(ctx context.Context, channelID string, ch *runtimeChannel) error {
	tried := map[string]struct{}{}
	var lastErr error
	for {
		currentNodeID := ch.currentNode.ID
		if currentNodeID != "" {
			tried[currentNodeID] = struct{}{}
		}
		occupiedExitIPs := m.currentExitIPsLocked()
		n, err := m.selectRotationNodeAvoidingLocked(ctx, ch.cfg, currentNodeID, tried, occupiedExitIPs)
		if err != nil {
			ch.lastRotationAt = time.Now()
			ch.err = fmt.Sprintf("no alternative node available for region %q", ch.cfg.Region)
			if lastErr != nil {
				return fmt.Errorf("%s: %w; no more candidates: %v", ch.err, lastErr, err)
			}
			return fmt.Errorf("%s: %w", ch.err, err)
		}
		tried[n.ID] = struct{}{}
		if n.ID == currentNodeID {
			now := time.Now()
			ch.lastRotationAt = now
			ch.err = fmt.Sprintf("no alternative node available for region %q", ch.cfg.Region)
			return fmt.Errorf("%s", ch.err)
		}
		if err := ch.tunnel.Switch(ctx, n); err != nil {
			ch.lastRotationAt = time.Now()
			ch.err = err.Error()
			lastErr = err
			continue
		}
		ch.lastNode = ch.currentNode
		ch.currentNode = n
		ch.startedAt = time.Now()
		ch.lastRotationAt = ch.startedAt
		ch.err = ""
		m.recordUse(ctx, channelID, n, ch.startedAt, ch.startedAt)
		return nil
	}
}

func (m *Manager) selectNode(ctx context.Context, ch config.Channel) (node.Node, error) {
	if m.cfg.Nodes == nil {
		return node.Node{}, fmt.Errorf("node store is required")
	}
	if ch.SelectionMode == SelectionManual {
		n, ok := m.findNode(ch.ManualNodeID)
		if !ok {
			return node.Node{}, fmt.Errorf("manual node %q not found", ch.ManualNodeID)
		}
		if !isAnyRegion(ch.Region) && n.Region != ch.Region {
			return node.Node{}, fmt.Errorf("manual node %q is region %q, channel requires %q", n.ID, n.Region, ch.Region)
		}
		return n, nil
	}
	n, ok := m.bestCheckedNode(ctx, ch.Region, "")
	if !ok {
		return node.Node{}, fmt.Errorf("no available node for region %q", ch.Region)
	}
	return n, nil
}

func isAnyRegion(region string) bool {
	region = strings.ToLower(strings.TrimSpace(region))
	return region == "" || region == "*"
}

func (m *Manager) selectRotationNode(ctx context.Context, ch config.Channel, currentNodeID string, occupiedExitIPs map[string]struct{}) (node.Node, error) {
	if m.cfg.Nodes == nil {
		return node.Node{}, fmt.Errorf("node store is required")
	}
	n, ok := m.bestRotationNode(ctx, ch, currentNodeID, occupiedExitIPs)
	if !ok {
		return node.Node{}, fmt.Errorf("no available node for region %q", ch.Region)
	}
	return n, nil
}

func (m *Manager) selectRotationNodeAvoidingLocked(ctx context.Context, ch config.Channel, currentNodeID string, avoided map[string]struct{}, occupiedExitIPs map[string]struct{}) (node.Node, error) {
	if m.cfg.Nodes == nil {
		return node.Node{}, fmt.Errorf("node store is required")
	}
	deepResults := m.deepTestResults(ctx)
	var best node.Node
	found := false
	for _, candidate := range m.cfg.Nodes.CandidatesByRegion(ch.Region, "", 0) {
		if candidate.ID == currentNodeID {
			continue
		}
		if ch.ManualNodeID != "" && candidate.ID == ch.ManualNodeID {
			continue
		}
		if _, ok := occupiedExitIPs[nodeExitIP(candidate)]; nodeExitIP(candidate) != "" && ok {
			continue
		}
		if _, ok := avoided[candidate.ID]; ok {
			continue
		}
		if !found || betterRotationNode(candidate, best, deepResults) {
			best = candidate
			found = true
		}
	}
	if !found {
		return node.Node{}, fmt.Errorf("no available node for region %q", ch.Region)
	}
	return best, nil
}

func (m *Manager) deepTestResults(ctx context.Context) map[string]deeptest.Result {
	if m.cfg.History == nil {
		return map[string]deeptest.Result{}
	}
	results, err := m.cfg.History.DeepTestResults(ctx)
	if err != nil {
		return map[string]deeptest.Result{}
	}
	return results
}

func (m *Manager) currentExitIPsLocked() map[string]struct{} {
	used := map[string]struct{}{}
	for _, ch := range m.channels {
		exitIP := nodeExitIP(ch.currentNode)
		if exitIP == "" {
			continue
		}
		used[exitIP] = struct{}{}
	}
	return used
}

func (m *Manager) bestRotationNode(ctx context.Context, ch config.Channel, currentNodeID string, occupiedExitIPs map[string]struct{}) (node.Node, bool) {
	if m.cfg.Nodes == nil {
		return node.Node{}, false
	}
	deepResults := m.deepTestResults(ctx)
	all := m.cfg.Nodes.CandidatesByRegion(ch.Region, "", 0)
	filtered := make([]node.Node, 0, len(all))
	for _, candidate := range all {
		if candidate.ID == currentNodeID {
			continue
		}
		if ch.ManualNodeID != "" && candidate.ID == ch.ManualNodeID {
			continue
		}
		if _, ok := occupiedExitIPs[nodeExitIP(candidate)]; nodeExitIP(candidate) != "" && ok {
			continue
		}
		filtered = append(filtered, candidate)
	}
	if len(filtered) == 0 {
		return node.Node{}, false
	}
	best := filtered[0]
	for _, candidate := range filtered[1:] {
		if betterRotationNode(candidate, best, deepResults) {
			best = candidate
		}
	}
	if best.ID == currentNodeID {
		return node.Node{}, false
	}
	return best, true
}

func (m *Manager) bestCheckedNode(ctx context.Context, region, avoidID string) (node.Node, bool) {
	if m.cfg.Nodes == nil {
		return node.Node{}, false
	}
	if m.cfg.NodeChecker == nil {
		return m.cfg.Nodes.BestByRegion(region, avoidID)
	}
	candidates := m.cfg.Nodes.CandidatesByRegion(region, avoidID, 8)
	if len(candidates) == 0 && avoidID != "" {
		candidates = m.cfg.Nodes.CandidatesByRegion(region, "", 8)
	}
	if len(candidates) == 0 {
		return node.Node{}, false
	}
	checked := make([]node.Node, 0, len(candidates))
	for _, candidate := range candidates {
		result := m.cfg.NodeChecker(ctx, candidate)
		m.cfg.Nodes.Update(candidate.ID, func(node.Node) node.Node { return result })
		if result.Available && result.ProbeStatus != "unknown" && result.LatencyMS > 0 {
			checked = append(checked, result)
		}
	}
	if len(checked) > 0 {
		best := checked[0]
		for _, candidate := range checked[1:] {
			if betterCheckedNode(candidate, best) {
				best = candidate
			}
		}
		return best, true
	}
	return m.cfg.Nodes.BestByRegion(region, avoidID)
}

func betterCheckedNode(a, b node.Node) bool {
	if a.LatencyMS != b.LatencyMS {
		return a.LatencyMS < b.LatencyMS
	}
	return a.Speed > b.Speed
}

func betterRotationNode(a, b node.Node, deepResults map[string]deeptest.Result) bool {
	aResult, aHas := deepResults[a.ID]
	bResult, bHas := deepResults[b.ID]
	aSuccess := aHas && aResult.Status == deeptest.StatusSuccess
	bSuccess := bHas && bResult.Status == deeptest.StatusSuccess
	if aSuccess != bSuccess {
		return aSuccess
	}
	if ap, bp := probePriority(a), probePriority(b); ap != bp {
		return ap > bp
	}
	if aSuccess && bSuccess && aResult.ConnectMS != bResult.ConnectMS {
		return aResult.ConnectMS < bResult.ConnectMS
	}
	return betterCheckedNode(a, b)
}

func probePriority(n node.Node) int {
	switch n.ProbeStatus {
	case "unknown":
		return 1
	case "unavailable":
		return 0
	default:
		return 2
	}
}

func (m *Manager) recordUse(ctx context.Context, channelID string, n node.Node, connectedAt time.Time, switchedAt time.Time) {
	if m.cfg.History == nil || channelID == "" || n.ID == "" {
		return
	}
	_ = m.cfg.History.RecordChannelNodeUse(ctx, NodeUse{
		ChannelID:   channelID,
		NodeID:      n.ID,
		ExitIP:      firstNonEmpty(n.IP, n.Hostname),
		ConnectedAt: connectedAt,
		SwitchedAt:  switchedAt,
	})
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
	nextRotationAt := time.Time{}
	if ch.cfg.Enabled && ch.cfg.SelectionMode == SelectionAuto && ch.cfg.RotateMinutes > 0 && !ch.startedAt.IsZero() {
		base := ch.lastRotationAt
		if base.IsZero() {
			base = ch.startedAt
		}
		nextRotationAt = base.Add(time.Duration(ch.cfg.RotateMinutes) * time.Minute)
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
		LastExitIP:     nodeExitIP(ch.lastNode),
		CurrentExitIP:  nodeExitIP(ch.currentNode),
		LastRotationAt: ch.lastRotationAt,
		NextRotationAt: nextRotationAt,
		TunnelStatus:   status,
		LastError:      ch.err,
		StartedAt:      ch.startedAt,
		ProxyURLHTTP:   fmt.Sprintf("http://%s:%d", ch.cfg.ListenHost, ch.cfg.ListenPort),
		ProxyURLSOCKS5: fmt.Sprintf("socks5://%s:%d", ch.cfg.ListenHost, ch.cfg.ListenPort),
	}
}

func nodeExitIP(n node.Node) string {
	if n.IP != "" {
		return n.IP
	}
	return n.Hostname
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
