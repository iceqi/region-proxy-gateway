package proxy

import (
	"bufio"
	"context"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/connection"
	"github.com/iceqi/region-proxy-gateway/internal/node"
	"github.com/iceqi/region-proxy-gateway/internal/session"
	"github.com/iceqi/region-proxy-gateway/internal/strategy"
	"github.com/iceqi/region-proxy-gateway/internal/tunnel"
)

func TestHTTPConnectWithoutAuthReturns407(t *testing.T) {
	server := newHTTPTestServer(&fakeSessionProvider{})
	client, proxy := net.Pipe()
	defer client.Close()

	go server.handleHTTP(proxy, 'C')

	if _, err := io.WriteString(client, "ONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n"); err != nil {
		t.Fatalf("write request: %v", err)
	}

	resp := readHTTPResponse(t, client)
	if resp.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusProxyAuthRequired)
	}
	if got := resp.Header.Get("Proxy-Authenticate"); got != `Basic realm="region-proxy-gateway"` {
		t.Fatalf("Proxy-Authenticate = %q, want Basic realm", got)
	}
}

func TestHTTPConnectWithValidAuthCreatesSessionAndReturns200(t *testing.T) {
	tun := &fakeTunnel{dialResult: make(chan fakeDial, 1)}
	provider := &fakeSessionProvider{sess: session.Session{Tunnel: tun}}
	server := newHTTPTestServer(provider)
	client, proxy := net.Pipe()
	defer client.Close()
	upstream, upstreamPeer := net.Pipe()
	defer upstreamPeer.Close()
	tun.dialResult <- fakeDial{conn: upstream}

	go server.handleHTTP(proxy, 'C')

	if _, err := io.WriteString(client, "ONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\nProxy-Authorization: "+basicAuth("jp-10", "secret")+"\r\n\r\n"); err != nil {
		t.Fatalf("write request: %v", err)
	}

	clientReader := bufio.NewReader(client)
	statusLine := readLine(t, clientReader)
	if statusLine != "HTTP/1.1 200 Connection Established\r\n" {
		t.Fatalf("status line = %q, want 200 Connection Established", statusLine)
	}
	if blank := readLine(t, clientReader); blank != "\r\n" {
		t.Fatalf("blank line = %q, want CRLF", blank)
	}
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

func TestHTTPConnectRelaysBytesThroughTunnelDialer(t *testing.T) {
	tun := &fakeTunnel{dialResult: make(chan fakeDial, 1)}
	server := newHTTPTestServer(&fakeSessionProvider{sess: session.Session{Tunnel: tun}})
	client, proxy := net.Pipe()
	defer client.Close()
	upstream, upstreamPeer := net.Pipe()
	defer upstreamPeer.Close()
	tun.dialResult <- fakeDial{conn: upstream}

	go server.handleHTTP(proxy, 'C')

	if _, err := io.WriteString(client, "ONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\nProxy-Authorization: "+basicAuth("jp-10", "secret")+"\r\n\r\n"); err != nil {
		t.Fatalf("write request: %v", err)
	}
	clientReader := bufio.NewReader(client)
	if statusLine := readLine(t, clientReader); statusLine != "HTTP/1.1 200 Connection Established\r\n" {
		t.Fatalf("status line = %q, want 200", statusLine)
	}
	if blank := readLine(t, clientReader); blank != "\r\n" {
		t.Fatalf("blank line = %q, want CRLF", blank)
	}

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

func TestPlainHTTPProxyRequestForwardsOriginForm(t *testing.T) {
	tun := &fakeTunnel{dialResult: make(chan fakeDial, 1)}
	server := newHTTPTestServer(&fakeSessionProvider{sess: session.Session{Tunnel: tun}})
	client, proxy := net.Pipe()
	defer client.Close()
	upstream, upstreamPeer := net.Pipe()
	defer upstreamPeer.Close()
	tun.dialResult <- fakeDial{conn: upstream}

	go server.handleHTTP(proxy, 'G')

	upstreamRequest := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(upstreamPeer)
		req, err := http.ReadRequest(reader)
		if err != nil {
			upstreamRequest <- "error: " + err.Error()
			return
		}
		if req.RequestURI != "/path?q=1" {
			upstreamRequest <- "request-uri: " + req.RequestURI
			return
		}
		if req.Header.Get("Proxy-Authorization") != "" {
			upstreamRequest <- "proxy-authorization leaked"
			return
		}
		if req.Header.Get("Proxy-Connection") != "" {
			upstreamRequest <- "proxy-connection leaked"
			return
		}
		_, _ = io.WriteString(upstreamPeer, "HTTP/1.1 204 No Content\r\nContent-Length: 0\r\n\r\n")
		upstreamRequest <- "ok"
	}()

	if _, err := io.WriteString(client, "ET http://example.com/path?q=1 HTTP/1.1\r\nHost: example.com\r\nProxy-Authorization: "+basicAuth("jp-10", "secret")+"\r\nProxy-Connection: keep-alive\r\n\r\n"); err != nil {
		t.Fatalf("write request: %v", err)
	}

	resp := readHTTPResponse(t, client)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	select {
	case got := <-upstreamRequest:
		if got != "ok" {
			t.Fatal(got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for upstream request")
	}
	if tun.gotAddress != "example.com:80" {
		t.Fatalf("dial address = %q, want example.com:80", tun.gotAddress)
	}
}

func TestPlainHTTPProxyResponseDoesNotWaitForClientClose(t *testing.T) {
	tun := &fakeTunnel{dialResult: make(chan fakeDial, 1)}
	server := newHTTPTestServer(&fakeSessionProvider{sess: session.Session{Tunnel: tun}})
	client, proxy := net.Pipe()
	defer client.Close()
	upstream, upstreamPeer := net.Pipe()
	defer upstreamPeer.Close()
	tun.dialResult <- fakeDial{conn: upstream}

	go server.handleHTTP(proxy, 'G')

	go func() {
		reader := bufio.NewReader(upstreamPeer)
		_, err := http.ReadRequest(reader)
		if err != nil {
			return
		}
		_, _ = io.WriteString(upstreamPeer, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
	}()

	if _, err := io.WriteString(client, "ET http://example.com/ HTTP/1.1\r\nHost: example.com\r\nProxy-Authorization: "+basicAuth("jp-10", "secret")+"\r\n\r\n"); err != nil {
		t.Fatalf("write request: %v", err)
	}

	done := make(chan *http.Response, 1)
	go func() {
		done <- readHTTPResponse(t, client)
	}()

	select {
	case resp := <-done:
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for plain HTTP response")
	}
}

func newHTTPTestServer(provider SessionProvider) *Server {
	return NewServer("127.0.0.1:0", "secret", []string{"jp"}, []int{10}, provider, connection.NewTracker())
}

func basicAuth(username, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
}

func readHTTPResponse(t *testing.T, conn net.Conn) *http.Response {
	t.Helper()
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return resp
}

func readLine(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read line: %v", err)
	}
	return line
}

type fakeSessionProvider struct {
	mu          sync.Mutex
	sess        session.Session
	err         error
	calls       int
	gotStrategy strategy.Strategy
}

func (p *fakeSessionProvider) GetOrCreate(ctx context.Context, strat strategy.Strategy) (session.Session, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.gotStrategy = strat
	return p.sess, p.err
}

type fakeDial struct {
	conn net.Conn
	err  error
}

type fakeTunnel struct {
	dialResult chan fakeDial
	gotNetwork string
	gotAddress string
}

func (t *fakeTunnel) Start(ctx context.Context, n node.Node, opts tunnel.Options) error {
	return nil
}

func (t *fakeTunnel) Stop(ctx context.Context) error {
	return nil
}

func (t *fakeTunnel) Switch(ctx context.Context, n node.Node) error {
	return nil
}

func (t *fakeTunnel) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	t.gotNetwork = network
	t.gotAddress = address
	if t.dialResult == nil {
		return nil, context.Canceled
	}
	result := <-t.dialResult
	return result.conn, result.err
}

func (t *fakeTunnel) Status() tunnel.Status {
	return tunnel.Status{}
}
