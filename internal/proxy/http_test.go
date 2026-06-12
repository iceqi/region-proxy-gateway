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
)

func TestHTTPConnectWithoutAuthReturns407(t *testing.T) {
	server := newHTTPTestServer(&fakeDialer{})
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
	dialer := &fakeDialer{dialResult: make(chan fakeDial, 1)}
	server := newHTTPTestServer(dialer)
	client, proxy := net.Pipe()
	defer client.Close()
	upstream, upstreamPeer := net.Pipe()
	defer upstreamPeer.Close()
	dialer.dialResult <- fakeDial{conn: upstream}

	go server.handleHTTP(proxy, 'C')

	if _, err := io.WriteString(client, "ONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\nProxy-Authorization: "+basicAuth("proxy", "secret")+"\r\n\r\n"); err != nil {
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

	if dialer.calls != 1 {
		t.Fatalf("dialer calls = %d, want 1", dialer.calls)
	}
	if dialer.gotChannelID != "jp-3000" {
		t.Fatalf("channel id = %q, want jp-3000", dialer.gotChannelID)
	}
	if dialer.gotNetwork != "tcp" {
		t.Fatalf("dial network = %q, want tcp", dialer.gotNetwork)
	}
	if dialer.gotAddress != "example.com:443" {
		t.Fatalf("dial address = %q, want example.com:443", dialer.gotAddress)
	}
}

func TestHTTPConnectRelaysBytesThroughTunnelDialer(t *testing.T) {
	dialer := &fakeDialer{dialResult: make(chan fakeDial, 1)}
	server := newHTTPTestServer(dialer)
	client, proxy := net.Pipe()
	defer client.Close()
	upstream, upstreamPeer := net.Pipe()
	defer upstreamPeer.Close()
	dialer.dialResult <- fakeDial{conn: upstream}

	go server.handleHTTP(proxy, 'C')

	if _, err := io.WriteString(client, "ONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\nProxy-Authorization: "+basicAuth("proxy", "secret")+"\r\n\r\n"); err != nil {
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

func TestHTTPConnectRelaysBufferedBytesAfterHeaders(t *testing.T) {
	dialer := &fakeDialer{dialResult: make(chan fakeDial, 1)}
	server := newHTTPTestServer(dialer)
	client, proxy := net.Pipe()
	defer client.Close()
	upstream, upstreamPeer := net.Pipe()
	defer upstreamPeer.Close()
	dialer.dialResult <- fakeDial{conn: upstream}

	go server.handleHTTP(proxy, 'C')

	if _, err := io.WriteString(client, "ONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\nProxy-Authorization: "+basicAuth("proxy", "secret")+"\r\n\r\nhello"); err != nil {
		t.Fatalf("write request: %v", err)
	}
	clientReader := bufio.NewReader(client)
	if statusLine := readLine(t, clientReader); statusLine != "HTTP/1.1 200 Connection Established\r\n" {
		t.Fatalf("status line = %q, want 200", statusLine)
	}
	if blank := readLine(t, clientReader); blank != "\r\n" {
		t.Fatalf("blank line = %q, want CRLF", blank)
	}

	buf := make([]byte, len("hello"))
	if _, err := io.ReadFull(upstreamPeer, buf); err != nil {
		t.Fatalf("upstream read buffered bytes: %v", err)
	}
	if string(buf) != "hello" {
		t.Fatalf("upstream read = %q, want hello", string(buf))
	}
}

func TestPlainHTTPProxyRequestForwardsOriginForm(t *testing.T) {
	dialer := &fakeDialer{dialResult: make(chan fakeDial, 1)}
	server := newHTTPTestServer(dialer)
	client, proxy := net.Pipe()
	defer client.Close()
	upstream, upstreamPeer := net.Pipe()
	defer upstreamPeer.Close()
	dialer.dialResult <- fakeDial{conn: upstream}

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

	if _, err := io.WriteString(client, "ET http://example.com/path?q=1 HTTP/1.1\r\nHost: example.com\r\nProxy-Authorization: "+basicAuth("proxy", "secret")+"\r\nProxy-Connection: keep-alive\r\n\r\n"); err != nil {
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
	if dialer.gotAddress != "example.com:80" {
		t.Fatalf("dial address = %q, want example.com:80", dialer.gotAddress)
	}
}

func TestPlainHTTPProxyResponseDoesNotWaitForClientClose(t *testing.T) {
	dialer := &fakeDialer{dialResult: make(chan fakeDial, 1)}
	server := newHTTPTestServer(dialer)
	client, proxy := net.Pipe()
	defer client.Close()
	upstream, upstreamPeer := net.Pipe()
	defer upstreamPeer.Close()
	dialer.dialResult <- fakeDial{conn: upstream}

	go server.handleHTTP(proxy, 'G')

	go func() {
		reader := bufio.NewReader(upstreamPeer)
		_, err := http.ReadRequest(reader)
		if err != nil {
			return
		}
		_, _ = io.WriteString(upstreamPeer, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
	}()

	if _, err := io.WriteString(client, "ET http://example.com/ HTTP/1.1\r\nHost: example.com\r\nProxy-Authorization: "+basicAuth("proxy", "secret")+"\r\n\r\n"); err != nil {
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

func TestPlainHTTPProxyKeepAliveResponseFinishesTracking(t *testing.T) {
	dialer := &fakeDialer{dialResult: make(chan fakeDial, 1)}
	tracker := connection.NewTracker()
	server := NewServer("127.0.0.1:0", "jp-3000", "proxy", "secret", dialer, tracker)
	client, proxy := net.Pipe()
	defer client.Close()
	upstream, upstreamPeer := net.Pipe()
	defer upstreamPeer.Close()
	dialer.dialResult <- fakeDial{conn: upstream}

	done := make(chan struct{}, 1)
	go func() {
		server.handleHTTP(proxy, 'G')
		done <- struct{}{}
	}()

	go func() {
		reader := bufio.NewReader(upstreamPeer)
		_, err := http.ReadRequest(reader)
		if err != nil {
			return
		}
		_, _ = io.WriteString(upstreamPeer, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: keep-alive\r\n\r\nok")
	}()

	if _, err := io.WriteString(client, "ET http://example.com/ HTTP/1.1\r\nHost: example.com\r\nProxy-Authorization: "+basicAuth("proxy", "secret")+"\r\n\r\n"); err != nil {
		t.Fatalf("write request: %v", err)
	}

	resp := readHTTPResponse(t, client)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q, want ok", string(body))
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for handler to finish")
	}
	if tracker.ActiveCount() != 0 {
		t.Fatalf("active connections = %d, want 0", tracker.ActiveCount())
	}
}

func TestPlainHTTPProxyTracksUploadBytes(t *testing.T) {
	dialer := &fakeDialer{dialResult: make(chan fakeDial, 1)}
	tracker := connection.NewTracker()
	server := NewServer("127.0.0.1:0", "jp-3000", "proxy", "secret", dialer, tracker)
	client, proxy := net.Pipe()
	defer client.Close()
	upstream, upstreamPeer := net.Pipe()
	defer upstreamPeer.Close()
	dialer.dialResult <- fakeDial{conn: upstream}

	go server.handleHTTP(proxy, 'P')

	done := make(chan connection.Record, 1)
	go func() {
		reader := bufio.NewReader(upstreamPeer)
		req, err := http.ReadRequest(reader)
		if err != nil {
			return
		}
		body, err := io.ReadAll(req.Body)
		if err != nil || string(body) != "payload" {
			return
		}
		_, _ = io.WriteString(upstreamPeer, "HTTP/1.1 204 No Content\r\nContent-Length: 0\r\n\r\n")

		deadline := time.After(time.Second)
		for {
			select {
			case <-deadline:
				return
			default:
			}
			records := tracker.List()
			if len(records) == 1 && records[0].BytesUp > 0 {
				done <- records[0]
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	if _, err := io.WriteString(client, "OST http://example.com/upload HTTP/1.1\r\nHost: example.com\r\nProxy-Authorization: "+basicAuth("proxy", "secret")+"\r\nContent-Length: 7\r\n\r\npayload"); err != nil {
		t.Fatalf("write request: %v", err)
	}
	resp := readHTTPResponse(t, client)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	select {
	case record := <-done:
		if record.BytesUp == 0 {
			t.Fatal("expected bytes_up to be tracked")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for upload byte tracking")
	}
}

func newHTTPTestServer(dialer Dialer) *Server {
	return NewServer("127.0.0.1:0", "jp-3000", "proxy", "secret", dialer, connection.NewTracker())
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

type fakeDial struct {
	conn net.Conn
	err  error
}

type fakeDialer struct {
	mu           sync.Mutex
	dialResult   chan fakeDial
	calls        int
	gotChannelID string
	gotNetwork   string
	gotAddress   string
}

func (d *fakeDialer) DialContext(ctx context.Context, channelID, network, address string) (net.Conn, error) {
	d.mu.Lock()
	d.calls++
	d.gotChannelID = channelID
	d.gotNetwork = network
	d.gotAddress = address
	d.mu.Unlock()
	if d.dialResult == nil {
		return nil, context.Canceled
	}
	result := <-d.dialResult
	return result.conn, result.err
}
