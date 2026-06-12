package proxy

import (
	"net"
	"testing"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/connection"
)

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
