package proxy

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/connection"
)

func TestServerAuthenticate(t *testing.T) {
	server := NewServer(
		"127.0.0.1:0",
		"secret",
		[]string{"jp", "us"},
		[]int{0, 10},
		nil,
		connection.NewTracker(),
	)

	tests := []struct {
		name      string
		username  string
		password  string
		wantError bool
	}{
		{
			name:      "valid strategy credentials",
			username:  "jp-10",
			password:  "secret",
			wantError: false,
		},
		{
			name:      "wrong password",
			username:  "jp-10",
			password:  "wrong",
			wantError: true,
		},
		{
			name:      "disallowed region",
			username:  "kr-10",
			password:  "secret",
			wantError: true,
		},
		{
			name:      "disallowed rotation minutes",
			username:  "jp-5",
			password:  "secret",
			wantError: true,
		},
		{
			name:      "invalid username",
			username:  "invalid",
			password:  "secret",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := server.authenticate(tt.username, tt.password)
			if tt.wantError {
				if err == nil {
					t.Fatal("authenticate returned nil error")
				}
				return
			}
			if err != nil {
				t.Fatalf("authenticate returned error: %v", err)
			}
			if got.Region != "jp" {
				t.Fatalf("region = %q, want %q", got.Region, "jp")
			}
			if got.RotateMinutes != 10 {
				t.Fatalf("rotate minutes = %d, want %d", got.RotateMinutes, 10)
			}
		})
	}
}

func TestServerDispatchesSOCKS5ByFirstByte(t *testing.T) {
	server := NewServer("127.0.0.1:0", "secret", nil, nil, nil, connection.NewTracker())
	dispatched := make(chan byte, 1)
	server.socks5Handler = func(conn net.Conn) {
		defer conn.Close()
		dispatched <- 0x05
	}
	server.httpHandler = func(conn net.Conn, firstByte byte) {
		defer conn.Close()
		t.Errorf("unexpected HTTP dispatch with first byte %q", firstByte)
	}

	client, upstream := net.Pipe()
	defer client.Close()

	go server.handleConn(upstream)

	if _, err := client.Write([]byte{0x05}); err != nil {
		t.Fatalf("write first byte: %v", err)
	}

	select {
	case got := <-dispatched:
		if got != 0x05 {
			t.Fatalf("dispatched byte = %#x, want 0x05", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for SOCKS5 dispatch")
	}
}

func TestServerDispatchesHTTPByFirstByte(t *testing.T) {
	server := NewServer("127.0.0.1:0", "secret", nil, nil, nil, connection.NewTracker())
	dispatched := make(chan byte, 1)
	server.socks5Handler = func(conn net.Conn) {
		defer conn.Close()
		t.Error("unexpected SOCKS5 dispatch")
	}
	server.httpHandler = func(conn net.Conn, firstByte byte) {
		defer conn.Close()
		dispatched <- firstByte
	}

	client, upstream := net.Pipe()
	defer client.Close()

	go server.handleConn(upstream)

	if _, err := client.Write([]byte{'G'}); err != nil {
		t.Fatalf("write first byte: %v", err)
	}

	select {
	case got := <-dispatched:
		if got != 'G' {
			t.Fatalf("first byte = %q, want %q", got, 'G')
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for HTTP dispatch")
	}
}

func TestServerServeReturnsWhenListenerCloses(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	server := NewServer(listener.Addr().String(), "secret", nil, nil, nil, connection.NewTracker())
	done := make(chan error, 1)
	go func() {
		done <- server.Serve(listener)
	}()

	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned error after listener close: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Serve to exit")
	}
}

func TestServerServeReturnsUnexpectedAcceptError(t *testing.T) {
	wantErr := errors.New("accept failed")
	server := NewServer("127.0.0.1:0", "secret", nil, nil, nil, connection.NewTracker())
	listener := &failingListener{err: &net.OpError{Op: "accept", Net: "tcp", Err: wantErr}}

	err := server.Serve(listener)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Serve error = %v, want %v", err, wantErr)
	}
}

func TestServerClosesIdleConnectionAfterHandshakeTimeout(t *testing.T) {
	server := NewServer("127.0.0.1:0", "secret", nil, nil, nil, connection.NewTracker())
	server.HandshakeTimeout = 20 * time.Millisecond
	client, upstream := net.Pipe()
	defer client.Close()

	done := make(chan struct{}, 1)
	go func() {
		server.handleConn(upstream)
		done <- struct{}{}
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for idle connection to close")
	}
}

type failingListener struct {
	err error
}

func (l *failingListener) Accept() (net.Conn, error) {
	return nil, l.err
}

func (l *failingListener) Close() error {
	return nil
}

func (l *failingListener) Addr() net.Addr {
	return &net.TCPAddr{}
}
