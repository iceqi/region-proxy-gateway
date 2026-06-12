package proxy

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/iceqi/region-proxy-gateway/internal/strategy"
)

const proxyAuthRealm = `Basic realm="region-proxy-gateway"`

func (s *Server) handleHTTP(conn net.Conn, firstByte byte) {
	defer conn.Close()

	reader := bufio.NewReader(io.MultiReader(bytes.NewReader([]byte{firstByte}), conn))
	req, err := http.ReadRequest(reader)
	if err != nil {
		writeHTTPError(conn, http.StatusBadRequest)
		return
	}

	strat, ok := s.authenticateHTTPRequest(req)
	if !ok {
		writeProxyAuthRequired(conn)
		return
	}

	if req.Method == http.MethodConnect {
		s.handleHTTPConnect(conn, reader, req, strat)
		return
	}
	s.handlePlainHTTP(conn, req, strat)
}

func (s *Server) authenticateHTTPRequest(req *http.Request) (strategy.Strategy, bool) {
	credentials, ok := ParseBasicProxyAuthorization(req.Header.Get("Proxy-Authorization"))
	if !ok {
		return strategy.Strategy{}, false
	}
	strat, err := s.authenticate(credentials.Username, credentials.Password)
	if err != nil {
		return strategy.Strategy{}, false
	}
	return strat, true
}

func (s *Server) handleHTTPConnect(client net.Conn, reader *bufio.Reader, req *http.Request, strat strategy.Strategy) {
	target := req.Host
	if target == "" {
		writeHTTPError(client, http.StatusBadRequest)
		return
	}

	upstream, ok := s.dialHTTPUpstream(client, req.Context(), strat, target)
	if !ok {
		return
	}
	defer upstream.Close()

	if _, err := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}

	s.trackAndRelay(client, upstream, reader, strat.Key(), target)
}

func (s *Server) handlePlainHTTP(client net.Conn, req *http.Request, strat strategy.Strategy) {
	target, err := proxyRequestTarget(req)
	if err != nil {
		writeHTTPError(client, http.StatusBadRequest)
		return
	}

	upstream, ok := s.dialHTTPUpstream(client, req.Context(), strat, target)
	if !ok {
		return
	}
	defer upstream.Close()

	req.RequestURI = ""
	req.Header.Del("Proxy-Authorization")
	req.Header.Del("Proxy-Connection")
	req.URL.Scheme = ""
	req.URL.Host = ""

	if err := req.Write(upstream); err != nil {
		return
	}

	s.trackHTTPResponse(client, upstream, strat.Key(), target)
}

func (s *Server) dialHTTPUpstream(client net.Conn, ctx context.Context, strat strategy.Strategy, target string) (net.Conn, bool) {
	if s.sessions == nil {
		writeHTTPError(client, http.StatusBadGateway)
		return nil, false
	}
	sess, err := s.sessions.GetOrCreate(ctx, strat)
	if err != nil {
		writeHTTPError(client, http.StatusBadGateway)
		return nil, false
	}
	if sess.Tunnel == nil {
		writeHTTPError(client, http.StatusBadGateway)
		return nil, false
	}
	upstream, err := sess.Tunnel.DialContext(ctx, "tcp", target)
	if err != nil {
		writeHTTPError(client, http.StatusBadGateway)
		return nil, false
	}
	return upstream, true
}

func (s *Server) trackAndRelay(client net.Conn, upstream net.Conn, buffered *bufio.Reader, strategyKey string, target string) {
	clientAddr := ""
	if client.RemoteAddr() != nil {
		clientAddr = client.RemoteAddr().String()
	}

	var id string
	if s.connections != nil {
		id = s.connections.Start(clientAddr, "http", strategyKey, target)
	}
	up, down := relay(client, upstream, buffered)
	if s.connections != nil {
		s.connections.AddBytes(id, up, down)
		s.connections.Finish(id)
	}
}

func (s *Server) trackHTTPResponse(client net.Conn, upstream net.Conn, strategyKey string, target string) {
	clientAddr := ""
	if client.RemoteAddr() != nil {
		clientAddr = client.RemoteAddr().String()
	}

	var id string
	if s.connections != nil {
		id = s.connections.Start(clientAddr, "http", strategyKey, target)
	}
	resp, err := http.ReadResponse(bufio.NewReader(upstream), nil)
	if err != nil {
		if s.connections != nil {
			s.connections.Finish(id)
		}
		return
	}
	counter := &countingWriter{writer: client}
	err = resp.Write(counter)
	if s.connections != nil {
		s.connections.AddBytes(id, 0, counter.count)
		s.connections.Finish(id)
	}
}

type countingWriter struct {
	writer io.Writer
	count  int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	w.count += int64(n)
	return n, err
}

func relay(client net.Conn, upstream net.Conn, buffered *bufio.Reader) (int64, int64) {
	var wg sync.WaitGroup
	var up int64
	var down int64

	wg.Add(2)
	go func() {
		defer wg.Done()
		var src io.Reader = client
		if buffered != nil && buffered.Buffered() > 0 {
			src = io.MultiReader(buffered, client)
		}
		up, _ = io.Copy(upstream, src)
		closeWrite(upstream)
		closeRead(client)
	}()
	go func() {
		defer wg.Done()
		down, _ = io.Copy(client, upstream)
		closeWrite(client)
		closeRead(upstream)
	}()
	wg.Wait()

	return up, down
}

func closeWrite(conn net.Conn) {
	if c, ok := conn.(interface{ CloseWrite() error }); ok {
		_ = c.CloseWrite()
		return
	}
	_ = conn.Close()
}

func closeRead(conn net.Conn) {
	if c, ok := conn.(interface{ CloseRead() error }); ok {
		_ = c.CloseRead()
	}
}

func proxyRequestTarget(req *http.Request) (string, error) {
	if req.URL == nil || req.URL.Host == "" {
		return "", fmt.Errorf("proxy request missing absolute URL host")
	}

	target := req.URL.Host
	if _, _, err := net.SplitHostPort(target); err == nil {
		return target, nil
	}
	if !strings.Contains(target, ":") || strings.HasPrefix(target, "[") {
		host := strings.TrimPrefix(strings.TrimSuffix(target, "]"), "[")
		switch req.URL.Scheme {
		case "http", "":
			return net.JoinHostPort(host, "80"), nil
		case "https":
			return net.JoinHostPort(host, "443"), nil
		default:
			return "", fmt.Errorf("unsupported proxy request scheme %q", req.URL.Scheme)
		}
	}
	return "", fmt.Errorf("invalid proxy request target %q", target)
}

func writeProxyAuthRequired(w io.Writer) {
	_, _ = fmt.Fprintf(w, "HTTP/1.1 %d %s\r\nProxy-Authenticate: %s\r\nContent-Length: 0\r\n\r\n", http.StatusProxyAuthRequired, http.StatusText(http.StatusProxyAuthRequired), proxyAuthRealm)
}

func writeHTTPError(w io.Writer, status int) {
	_, _ = fmt.Fprintf(w, "HTTP/1.1 %d %s\r\nContent-Length: 0\r\n\r\n", status, http.StatusText(status))
}
