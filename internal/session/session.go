package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/node"
	"github.com/iceqi/region-proxy-gateway/internal/strategy"
	"github.com/iceqi/region-proxy-gateway/internal/tunnel"
)

type Factory func(key string) tunnel.Tunnel

type Session struct {
	Strategy   strategy.Strategy `json:"strategy"`
	Node       node.Node         `json:"node"`
	CreatedAt  time.Time         `json:"created_at"`
	LastUsedAt time.Time         `json:"last_used_at"`
	Tunnel     tunnel.Tunnel     `json:"-"`
}

type Manager struct {
	mu         sync.RWMutex
	nodes      *node.Store
	maxActive  int
	factory    Factory
	sessions   map[string]*Session
	creating   map[string]struct{}
	nextDevice int
}

func NewManager(nodes *node.Store, maxActive int, factory Factory) *Manager {
	return &Manager{
		nodes:     nodes,
		maxActive: maxActive,
		factory:   factory,
		sessions:  make(map[string]*Session),
		creating:  make(map[string]struct{}),
	}
}

func (m *Manager) GetOrCreate(ctx context.Context, strat strategy.Strategy) (*Session, error) {
	key := strat.Key()

	m.mu.Lock()
	if sess, ok := m.sessions[key]; ok {
		sess.LastUsedAt = time.Now()
		m.mu.Unlock()
		return sess, nil
	}
	if _, ok := m.creating[key]; ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("session %q is already being created", key)
	}
	if m.maxActive > 0 && len(m.sessions) >= m.maxActive {
		m.mu.Unlock()
		return nil, fmt.Errorf("max active sessions reached")
	}
	device := deviceName(m.nextDevice)
	m.nextDevice++
	m.creating[key] = struct{}{}
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.creating, key)
		m.mu.Unlock()
	}()

	selected, ok := m.nodes.BestByRegion(strat.Region, "")
	if !ok {
		return nil, fmt.Errorf("no available node for region %q", strat.Region)
	}

	tun := m.factory(key)
	if tun == nil {
		return nil, fmt.Errorf("session tunnel factory returned nil for %q", key)
	}
	if err := tun.Start(ctx, selected, tunnel.Options{Name: key, DeviceName: device}); err != nil {
		return nil, fmt.Errorf("start tunnel for %q: %w", key, err)
	}

	now := time.Now()
	sess := &Session{
		Strategy:   strat,
		Node:       selected,
		CreatedAt:  now,
		LastUsedAt: now,
		Tunnel:     tun,
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.sessions[key]; ok {
		_ = tun.Stop(context.Background())
		existing.LastUsedAt = time.Now()
		return existing, nil
	}
	m.sessions[key] = sess
	return sess, nil
}

func (m *Manager) SwitchNow(ctx context.Context, key string) error {
	m.mu.Lock()
	sess, ok := m.sessions[key]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("session %q not found", key)
	}
	currentNode := sess.Node
	region := sess.Strategy.Region
	tun := sess.Tunnel
	m.mu.Unlock()

	next, ok := m.nodes.BestByRegion(region, currentNode.ID)
	if !ok {
		return fmt.Errorf("no available node for region %q", region)
	}
	if next.ID == currentNode.ID {
		return fmt.Errorf("no alternative node for region %q", region)
	}
	if err := tun.Switch(ctx, next); err != nil {
		return fmt.Errorf("switch tunnel for %q: %w", key, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok = m.sessions[key]
	if !ok {
		return fmt.Errorf("session %q not found after switch", key)
	}
	sess.Node = next
	sess.LastUsedAt = time.Now()
	return nil
}

func (m *Manager) List() []Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]Session, 0, len(m.sessions))
	for _, sess := range m.sessions {
		sessions = append(sessions, *sess)
	}
	return sessions
}

func (m *Manager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.sessions)
}

func deviceName(index int) string {
	return fmt.Sprintf("rpg%d", index)
}
