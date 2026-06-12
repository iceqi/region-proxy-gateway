package tunnel

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/node"
)

type Fake struct {
	mu     sync.RWMutex
	status Status
}

func NewFake(name string) *Fake {
	return &Fake{status: Status{Name: name}}
}

func (f *Fake) Start(ctx context.Context, n node.Node, opts Options) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = Status{Name: opts.Name, NodeID: n.ID, Ready: true, StartedAt: time.Now()}
	return nil
}

func (f *Fake) Stop(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status.Ready = false
	return nil
}

func (f *Fake) Switch(ctx context.Context, n node.Node) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status.NodeID = n.ID
	f.status.Ready = true
	f.status.StartedAt = time.Now()
	f.status.Error = ""
	return nil
}

func (f *Fake) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, network, address)
}

func (f *Fake) Status() Status {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.status
}
