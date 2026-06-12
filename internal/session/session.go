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
	Strategy   strategy.Strategy
	Node       node.Node
	CreatedAt  time.Time
	LastUsedAt time.Time
	Tunnel     tunnel.Tunnel
}

type Manager struct {
	mu        sync.RWMutex
	nodes     *node.Store
	maxActive int
	factory   Factory
	sessions  map[string]*Session
}

func NewManager(nodes *node.Store, maxActive int, factory Factory) *Manager {
	return &Manager{
		nodes:     nodes,
		maxActive: maxActive,
		factory:   factory,
		sessions:  make(map[string]*Session),
	}
}

func (m *Manager) GetOrCreate(ctx context.Context, strat strategy.Strategy) (*Session, error) {
	key := strat.Key()

	m.mu.Lock()
	defer m.mu.Unlock()

	if sess, ok := m.sessions[key]; ok {
		sess.LastUsedAt = time.Now()
		return sess, nil
	}
	if m.maxActive > 0 && len(m.sessions) >= m.maxActive {
		return nil, fmt.Errorf("max active sessions reached")
	}

	selected, ok := m.nodes.BestByRegion(strat.Region, "")
	if !ok {
		return nil, fmt.Errorf("no available node for region %q", strat.Region)
	}

	tun := m.factory(key)
	if tun == nil {
		return nil, fmt.Errorf("session tunnel factory returned nil for %q", key)
	}
	if err := tun.Start(ctx, selected, tunnel.Options{Name: key}); err != nil {
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
	m.sessions[key] = sess
	return sess, nil
}

func (m *Manager) SwitchNow(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, ok := m.sessions[key]
	if !ok {
		return fmt.Errorf("session %q not found", key)
	}

	next, ok := m.nodes.BestByRegion(sess.Strategy.Region, sess.Node.ID)
	if !ok {
		return fmt.Errorf("no available node for region %q", sess.Strategy.Region)
	}
	if next.ID == sess.Node.ID {
		return fmt.Errorf("no alternative node for region %q", sess.Strategy.Region)
	}
	if err := sess.Tunnel.Switch(ctx, next); err != nil {
		return fmt.Errorf("switch tunnel for %q: %w", key, err)
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
