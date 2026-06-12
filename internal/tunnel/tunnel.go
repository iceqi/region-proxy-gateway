package tunnel

import (
	"context"
	"net"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/node"
)

type Options struct {
	Name       string
	DataDir    string
	Command    string
	DeviceName string
}

type Status struct {
	Name      string    `json:"name"`
	NodeID    string    `json:"node_id"`
	Ready     bool      `json:"ready"`
	StartedAt time.Time `json:"started_at"`
	Error     string    `json:"error"`
	PID       int       `json:"pid,omitempty"`
}

type Tunnel interface {
	Start(ctx context.Context, n node.Node, opts Options) error
	Stop(ctx context.Context) error
	Switch(ctx context.Context, n node.Node) error
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
	Status() Status
}
