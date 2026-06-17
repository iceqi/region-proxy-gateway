package proxy

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	socks5Version               = 0x05
	socks5AuthUsernamePassword  = 0x02
	socks5NoAcceptableMethods   = 0xff
	socks5AuthVersion           = 0x01
	socks5AuthSuccess           = 0x00
	socks5AuthFailure           = 0x01
	socks5CommandConnect        = 0x01
	socks5ReplySuccess          = 0x00
	socks5ReplyGeneralFailure   = 0x01
	socks5ReplyCommandNoSupport = 0x07
	socks5AddrIPv4              = 0x01
	socks5AddrDomain            = 0x03
	socks5AddrIPv6              = 0x04
)

var (
	socks5SuccessResponse         = []byte{socks5Version, socks5ReplySuccess, 0x00, socks5AddrIPv4, 0, 0, 0, 0, 0, 0}
	socks5GeneralFailureResponse  = []byte{socks5Version, socks5ReplyGeneralFailure, 0x00, socks5AddrIPv4, 0, 0, 0, 0, 0, 0}
	socks5CommandRejectedResponse = []byte{socks5Version, socks5ReplyCommandNoSupport, 0x00, socks5AddrIPv4, 0, 0, 0, 0, 0, 0}
)

func (s *Server) handleSOCKS5(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	auth, ok := s.negotiateSOCKS5Auth(conn, reader)
	if !ok {
		return
	}

	command, target, ok := readSOCKS5Request(reader)
	if !ok {
		_, _ = conn.Write(socks5GeneralFailureResponse)
		return
	}
	if command != socks5CommandConnect {
		_, _ = conn.Write(socks5CommandRejectedResponse)
		return
	}
	_ = conn.SetReadDeadline(time.Time{})

	routeID := s.routeID(auth)
	upstream, ok := s.dialSOCKS5Upstream(conn, context.Background(), routeID, target)
	if !ok {
		return
	}
	defer upstream.Close()

	if _, err := conn.Write(socks5SuccessResponse); err != nil {
		return
	}

	s.trackAndRelaySOCKS5(conn, upstream, reader, routeID, target)
}

func (s *Server) negotiateSOCKS5Auth(conn net.Conn, reader *bufio.Reader) (authContext, bool) {
	methodCount, err := reader.ReadByte()
	if err != nil {
		return authContext{}, false
	}

	methods := make([]byte, int(methodCount))
	if _, err := io.ReadFull(reader, methods); err != nil {
		return authContext{}, false
	}

	if !bytesContain(methods, socks5AuthUsernamePassword) {
		_, _ = conn.Write([]byte{socks5Version, socks5NoAcceptableMethods})
		return authContext{}, false
	}
	if _, err := conn.Write([]byte{socks5Version, socks5AuthUsernamePassword}); err != nil {
		return authContext{}, false
	}

	username, password, ok := readSOCKS5UsernamePassword(reader)
	if !ok {
		_, _ = conn.Write([]byte{socks5AuthVersion, socks5AuthFailure})
		return authContext{}, false
	}

	auth, err := s.authenticate(username, password)
	if err != nil {
		_, _ = conn.Write([]byte{socks5AuthVersion, socks5AuthFailure})
		return authContext{}, false
	}
	if _, err := conn.Write([]byte{socks5AuthVersion, socks5AuthSuccess}); err != nil {
		return authContext{}, false
	}
	return auth, true
}

func readSOCKS5UsernamePassword(reader *bufio.Reader) (string, string, bool) {
	version, err := reader.ReadByte()
	if err != nil || version != socks5AuthVersion {
		return "", "", false
	}
	username, ok := readSOCKS5LengthPrefixedString(reader)
	if !ok {
		return "", "", false
	}
	password, ok := readSOCKS5LengthPrefixedString(reader)
	if !ok {
		return "", "", false
	}
	return username, password, true
}

func readSOCKS5Request(reader *bufio.Reader) (byte, string, bool) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(reader, header); err != nil {
		return 0, "", false
	}
	if header[0] != socks5Version || header[2] != 0x00 {
		return 0, "", false
	}

	host, ok := readSOCKS5Address(reader, header[3])
	if !ok {
		return header[1], "", false
	}
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(reader, portBytes); err != nil {
		return header[1], "", false
	}
	port := binary.BigEndian.Uint16(portBytes)
	return header[1], net.JoinHostPort(host, fmt.Sprintf("%d", port)), true
}

func readSOCKS5Address(reader *bufio.Reader, addressType byte) (string, bool) {
	switch addressType {
	case socks5AddrIPv4:
		raw := make([]byte, net.IPv4len)
		if _, err := io.ReadFull(reader, raw); err != nil {
			return "", false
		}
		return net.IP(raw).String(), true
	case socks5AddrDomain:
		host, ok := readSOCKS5LengthPrefixedString(reader)
		if !ok || host == "" {
			return "", false
		}
		return host, true
	case socks5AddrIPv6:
		raw := make([]byte, net.IPv6len)
		if _, err := io.ReadFull(reader, raw); err != nil {
			return "", false
		}
		return net.IP(raw).String(), true
	default:
		return "", false
	}
}

func readSOCKS5LengthPrefixedString(reader *bufio.Reader) (string, bool) {
	length, err := reader.ReadByte()
	if err != nil {
		return "", false
	}
	value := make([]byte, int(length))
	if _, err := io.ReadFull(reader, value); err != nil {
		return "", false
	}
	return string(value), true
}

func (s *Server) dialSOCKS5Upstream(client net.Conn, ctx context.Context, routeID string, target string) (net.Conn, bool) {
	if s.dialer == nil {
		_, _ = client.Write(socks5GeneralFailureResponse)
		return nil, false
	}
	upstream, err := s.dialer.DialContext(ctx, routeID, "tcp", target)
	if err != nil {
		_, _ = client.Write(socks5GeneralFailureResponse)
		return nil, false
	}
	return upstream, true
}

func (s *Server) trackAndRelaySOCKS5(client net.Conn, upstream net.Conn, buffered *bufio.Reader, channelID string, target string) {
	clientAddr := ""
	if client.RemoteAddr() != nil {
		clientAddr = client.RemoteAddr().String()
	}

	var id string
	if s.connections != nil {
		id = s.connections.Start(clientAddr, "socks5", channelID, target)
	}
	up, down := relay(client, upstream, buffered)
	if s.connections != nil {
		s.connections.AddBytes(id, up, down)
		s.connections.Finish(id)
	}
}

func bytesContain(items []byte, want byte) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
