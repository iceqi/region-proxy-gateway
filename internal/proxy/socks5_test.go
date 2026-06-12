package proxy

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/session"
)

func TestSOCKS5WithoutUsernamePasswordMethodIsRejected(t *testing.T) {
	server := newHTTPTestServer(&fakeSessionProvider{})
	client, proxy := net.Pipe()
	defer client.Close()

	go server.handleSOCKS5(proxy)

	writeSOCKS5Methods(t, client, 0x00)
	assertSOCKS5Response(t, client, []byte{0x05, 0xff})
}

func TestSOCKS5ValidAuthAndDomainConnectSucceeds(t *testing.T) {
	tun := &fakeTunnel{dialResult: make(chan fakeDial, 1)}
	provider := &fakeSessionProvider{sess: session.Session{Tunnel: tun}}
	server := newHTTPTestServer(provider)
	client, proxy := net.Pipe()
	defer client.Close()
	upstream, upstreamPeer := net.Pipe()
	defer upstreamPeer.Close()
	tun.dialResult <- fakeDial{conn: upstream}

	go server.handleSOCKS5(proxy)

	negotiateSOCKS5Auth(t, client, "jp-10", "secret", true)
	writeSOCKS5ConnectDomain(t, client, 0x01, "example.com", 443)
	assertSOCKS5Response(t, client, []byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	upstreamPeer.Close()

	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
	if provider.gotStrategy.Key() != "jp-10" {
		t.Fatalf("strategy = %q, want jp-10", provider.gotStrategy.Key())
	}
	if tun.gotNetwork != "tcp" {
		t.Fatalf("dial network = %q, want tcp", tun.gotNetwork)
	}
	if tun.gotAddress != "example.com:443" {
		t.Fatalf("dial address = %q, want example.com:443", tun.gotAddress)
	}
}

func TestSOCKS5ConnectSupportsIPv4AndIPv6(t *testing.T) {
	tests := []struct {
		name        string
		writeTarget func(t *testing.T, conn net.Conn)
		wantAddress string
	}{
		{
			name: "ipv4",
			writeTarget: func(t *testing.T, conn net.Conn) {
				writeSOCKS5ConnectIPv4(t, conn, 0x01, net.IPv4(192, 0, 2, 10), 8080)
			},
			wantAddress: "192.0.2.10:8080",
		},
		{
			name: "ipv6",
			writeTarget: func(t *testing.T, conn net.Conn) {
				writeSOCKS5ConnectIPv6(t, conn, 0x01, net.ParseIP("2001:db8::1"), 8443)
			},
			wantAddress: "[2001:db8::1]:8443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tun := &fakeTunnel{dialResult: make(chan fakeDial, 1)}
			provider := &fakeSessionProvider{sess: session.Session{Tunnel: tun}}
			server := newHTTPTestServer(provider)
			client, proxy := net.Pipe()
			defer client.Close()
			upstream, upstreamPeer := net.Pipe()
			defer upstreamPeer.Close()
			tun.dialResult <- fakeDial{conn: upstream}

			go server.handleSOCKS5(proxy)

			negotiateSOCKS5Auth(t, client, "jp-10", "secret", true)
			tt.writeTarget(t, client)
			assertSOCKS5Response(t, client, []byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
			upstreamPeer.Close()

			if tun.gotAddress != tt.wantAddress {
				t.Fatalf("dial address = %q, want %q", tun.gotAddress, tt.wantAddress)
			}
		})
	}
}

func TestSOCKS5InvalidPasswordIsRejected(t *testing.T) {
	server := newHTTPTestServer(&fakeSessionProvider{})
	client, proxy := net.Pipe()
	defer client.Close()

	go server.handleSOCKS5(proxy)

	negotiateSOCKS5Auth(t, client, "jp-10", "wrong", false)
}

func TestSOCKS5UnsupportedCommandIsRejected(t *testing.T) {
	server := newHTTPTestServer(&fakeSessionProvider{})
	client, proxy := net.Pipe()
	defer client.Close()

	go server.handleSOCKS5(proxy)

	negotiateSOCKS5Auth(t, client, "jp-10", "secret", true)
	writeSOCKS5ConnectDomain(t, client, 0x02, "example.com", 443)
	assertSOCKS5Response(t, client, []byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
}

func TestSOCKS5RelaysBytesThroughTunnelDialer(t *testing.T) {
	tun := &fakeTunnel{dialResult: make(chan fakeDial, 1)}
	server := newHTTPTestServer(&fakeSessionProvider{sess: session.Session{Tunnel: tun}})
	client, proxy := net.Pipe()
	defer client.Close()
	upstream, upstreamPeer := net.Pipe()
	defer upstreamPeer.Close()
	tun.dialResult <- fakeDial{conn: upstream}

	go server.handleSOCKS5(proxy)

	negotiateSOCKS5Auth(t, client, "jp-10", "secret", true)
	writeSOCKS5ConnectDomain(t, client, 0x01, "example.com", 443)
	assertSOCKS5Response(t, client, []byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

	clientRead := make(chan string, 1)
	go func() {
		buf := make([]byte, len("from-upstream"))
		_, err := io.ReadFull(client, buf)
		if err != nil {
			clientRead <- "error: " + err.Error()
			return
		}
		clientRead <- string(buf)
	}()

	if _, err := io.WriteString(client, "from-client"); err != nil {
		t.Fatalf("client write: %v", err)
	}
	buf := make([]byte, len("from-client"))
	if _, err := io.ReadFull(upstreamPeer, buf); err != nil {
		t.Fatalf("upstream read: %v", err)
	}
	if string(buf) != "from-client" {
		t.Fatalf("upstream read = %q, want from-client", string(buf))
	}

	if _, err := io.WriteString(upstreamPeer, "from-upstream"); err != nil {
		t.Fatalf("upstream write: %v", err)
	}
	select {
	case got := <-clientRead:
		if got != "from-upstream" {
			t.Fatalf("client read = %q, want from-upstream", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for downstream relay")
	}
}

func writeSOCKS5Methods(t *testing.T, conn net.Conn, methods ...byte) {
	t.Helper()
	req := append([]byte{byte(len(methods))}, methods...)
	if _, err := conn.Write(req); err != nil {
		t.Fatalf("write methods: %v", err)
	}
}

func negotiateSOCKS5Auth(t *testing.T, conn net.Conn, username, password string, wantOK bool) {
	t.Helper()
	writeSOCKS5Methods(t, conn, 0x00, 0x02)
	assertSOCKS5Response(t, conn, []byte{0x05, 0x02})

	req := []byte{0x01, byte(len(username))}
	req = append(req, []byte(username)...)
	req = append(req, byte(len(password)))
	req = append(req, []byte(password)...)
	if _, err := conn.Write(req); err != nil {
		t.Fatalf("write auth: %v", err)
	}

	if wantOK {
		assertSOCKS5Response(t, conn, []byte{0x01, 0x00})
		return
	}
	assertSOCKS5Response(t, conn, []byte{0x01, 0x01})
}

func writeSOCKS5ConnectDomain(t *testing.T, conn net.Conn, command byte, host string, port int) {
	t.Helper()
	req := []byte{0x05, command, 0x00, 0x03, byte(len(host))}
	req = append(req, []byte(host)...)
	req = append(req, byte(port>>8), byte(port))
	if _, err := conn.Write(req); err != nil {
		t.Fatalf("write connect: %v", err)
	}
}

func writeSOCKS5ConnectIPv4(t *testing.T, conn net.Conn, command byte, ip net.IP, port int) {
	t.Helper()
	raw := ip.To4()
	if raw == nil {
		t.Fatalf("invalid ipv4 %v", ip)
	}
	req := []byte{0x05, command, 0x00, 0x01}
	req = append(req, raw...)
	req = append(req, byte(port>>8), byte(port))
	if _, err := conn.Write(req); err != nil {
		t.Fatalf("write ipv4 connect: %v", err)
	}
}

func writeSOCKS5ConnectIPv6(t *testing.T, conn net.Conn, command byte, ip net.IP, port int) {
	t.Helper()
	raw := ip.To16()
	if raw == nil {
		t.Fatalf("invalid ipv6 %v", ip)
	}
	req := []byte{0x05, command, 0x00, 0x04}
	req = append(req, raw...)
	req = append(req, byte(port>>8), byte(port))
	if _, err := conn.Write(req); err != nil {
		t.Fatalf("write ipv6 connect: %v", err)
	}
}

func assertSOCKS5Response(t *testing.T, conn net.Conn, want []byte) {
	t.Helper()
	got := make([]byte, len(want))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("read response: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("response = %v, want %v", got, want)
	}
}
