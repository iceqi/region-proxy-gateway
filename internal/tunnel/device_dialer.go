package tunnel

import (
	"context"
	"net"
)

type DeviceDialer interface {
	DialContext(ctx context.Context, deviceName, network, address string) (net.Conn, error)
}

type SystemDeviceDialer struct{}
