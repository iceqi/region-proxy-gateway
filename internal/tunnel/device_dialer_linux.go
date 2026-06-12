//go:build linux

package tunnel

import (
	"context"
	"net"
	"syscall"
)

func (SystemDeviceDialer) DialContext(ctx context.Context, deviceName, network, address string) (net.Conn, error) {
	dialer := net.Dialer{
		Control: func(network, address string, conn syscall.RawConn) error {
			var controlErr error
			if err := conn.Control(func(fd uintptr) {
				controlErr = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, deviceName)
			}); err != nil {
				return err
			}
			return controlErr
		},
	}
	return dialer.DialContext(ctx, network, address)
}
