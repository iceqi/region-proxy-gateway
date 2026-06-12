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

type managedSession struct {
	mu        sync.Mutex
	session   Session
	switching bool
}

type createState struct {
	done chan struct{}
	sess Session
	err  error
}

type Manager struct {
	mu         sync.RWMutex
	nodes      *node.Store
	maxActive  int
	factory    Factory
	sessions   map[string]*managedSession
	creating   map[string]*createState
	nextDevice int
}

func NewManager(nodes *node.Store, maxActive int, factory Factory) *Manager {
	return &Manager{
		nodes:     nodes,
		maxActive: maxActive,
		factory:   factory,
		sessions:  make(map[string]*managedSession),
		creating:  make(map[string]*createState),
	}
}

func (m *Manager) GetOrCreate(ctx context.Context, strat strategy.Strategy) (Session, error) {
	key := strat.Key()

	m.mu.Lock()
	if entry, ok := m.sessions[key]; ok {
		m.mu.Unlock()
		entry.mu.Lock()
		defer entry.mu.Unlock()
		entry.session.LastUsedAt = time.Now()
		return entry.session, nil
	}
	if creating, ok := m.creating[key]; ok {
		m.mu.Unlock()
		select {
		case <-creating.done:
			return creating.sess, creating.err
		case <-ctx.Done():
			return Session{}, ctx.Err()
		}
	}
	if m.maxActive > 0 && len(m.sessions)+len(m.creating) >= m.maxActive {
		m.mu.Unlock()
		return Session{}, fmt.Errorf("max active sessions reached")
	}
	device := deviceName(m.nextDevice)
	m.nextDevice++
	creating := &createState{done: make(chan struct{})}
	m.creating[key] = creating
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.creating, key)
		m.mu.Unlock()
		close(creating.done)
	}()

	selected, ok := m.nodes.BestByRegion(strat.Region, "")
	if !ok {
		return m.finishCreate(creating, Session{}, fmt.Errorf("no available node for region %q", strat.Region))
	}

	tun := m.factory(key)
	if tun == nil {
		return m.finishCreate(creating, Session{}, fmt.Errorf("session tunnel factory returned nil for %q", key))
	}
	if err := tun.Start(ctx, selected, tunnel.Options{Name: key, DeviceName: device}); err != nil {
		return m.finishCreate(creating, Session{}, fmt.Errorf("start tunnel for %q: %w", key, err))
	}

	now := time.Now()
	sess := Session{
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
		existing.mu.Lock()
		defer existing.mu.Unlock()
		existing.session.LastUsedAt = time.Now()
		return m.finishCreate(creating, existing.session, nil)
	}
	m.sessions[key] = &managedSession{session: sess}
	return m.finishCreate(creating, sess, nil)
}

func (m *Manager) finishCreate(creating *createState, sess Session, err error) (Session, error) {
	creating.sess = sess
	creating.err = err
	return sess, err
}

func (m *Manager) SwitchNow(ctx context.Context, key string) error {
	m.mu.RLock()
	entry, ok := m.sessions[key]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session %q not found", key)
	}

	entry.mu.Lock()
	if entry.switching {
		entry.mu.Unlock()
		return fmt.Errorf("session %q is already switching", key)
	}
	entry.switching = true
	currentNode := entry.session.Node
	region := entry.session.Strategy.Region
	tun := entry.session.Tunnel
	entry.mu.Unlock()

	defer func() {
		entry.mu.Lock()
		entry.switching = false
		entry.mu.Unlock()
	}()

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

	entry.mu.Lock()
	defer entry.mu.Unlock()
	entry.session.Node = next
	entry.session.LastUsedAt = time.Now()
	return nil
}

func (m *Manager) List() []Session {
	m.mu.RLock()
	entries := make([]*managedSession, 0, len(m.sessions))
	for _, entry := range m.sessions {
		entries = append(entries, entry)
	}
	m.mu.RUnlock()

	sessions := make([]Session, 0, len(entries))
	for _, entry := range entries {
		entry.mu.Lock()
		sessions = append(sessions, entry.session)
		entry.mu.Unlock()
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
