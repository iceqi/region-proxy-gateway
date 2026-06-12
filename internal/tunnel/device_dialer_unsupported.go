//go:build !linux

package tunnel

import (
	"context"
	"fmt"
	"net"
	"runtime"
)

func (SystemDeviceDialer) DialContext(ctx context.Context, deviceName, network, address string) (net.Conn, error) {
	return nil, fmt.Errorf("device-bound dialing is only supported on linux, current os is %s", runtime.GOOS)
}
