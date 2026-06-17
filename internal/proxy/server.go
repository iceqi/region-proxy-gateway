package proxy

import (
	"context"
	"errors"
	"net"
	"os"
	"sync"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/connection"
)

type Dialer interface {
	DialContext(ctx context.Context, channelID, network, address string) (net.Conn, error)
}

type httpHandlerFunc func(conn net.Conn, firstByte byte)
type socks5HandlerFunc func(conn net.Conn)

type Server struct {
	ListenAddr       string
	ChannelID        string
	ProxyUsername    string
	ProxyPassword    string
	HandshakeTimeout time.Duration

	authMu      sync.RWMutex
	dialer      Dialer
	connections *connection.Tracker

	httpHandler   httpHandlerFunc
	socks5Handler socks5HandlerFunc
}

type authContext struct {
	Username string
	Region   string
}

func NewServer(listenAddr string, channelID string, proxyUsername string, proxyPassword string, dialer Dialer, connections *connection.Tracker) *Server {
	server := &Server{
		ListenAddr:    listenAddr,
		ChannelID:     channelID,
		ProxyUsername: proxyUsername,
		ProxyPassword: proxyPassword,
		dialer:        dialer,
		connections:   connections,
	}
	server.httpHandler = server.handleHTTP
	server.socks5Handler = server.handleSOCKS5
	return server
}

func (s *Server) SetCredentials(username, password string) {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	s.ProxyUsername = username
	s.ProxyPassword = password
}

func (s *Server) Serve(listener net.Listener) error {
	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			if errors.Is(err, os.ErrClosed) {
				return nil
			}
			return err
		}

		go s.handleConn(conn)
	}
}

func (s *Server) authenticate(username, password string) (authContext, error) {
	s.authMu.RLock()
	proxyUsername := s.ProxyUsername
	proxyPassword := s.ProxyPassword
	s.authMu.RUnlock()
	baseUsername, region := splitUsernameRegion(username, proxyUsername)
	if proxyUsername != "" && baseUsername != proxyUsername {
		return authContext{}, errors.New("invalid proxy credentials")
	}
	if !CheckPassword(password, proxyPassword) {
		return authContext{}, errors.New("invalid proxy credentials")
	}
	return authContext{Username: baseUsername, Region: region}, nil
}

func splitUsernameRegion(username, baseUsername string) (string, string) {
	for _, separator := range []string{"+", "-", "_"} {
		prefix := baseUsername + separator
		if baseUsername != "" && len(username) > len(prefix) && username[:len(prefix)] == prefix {
			return baseUsername, username[len(prefix):]
		}
	}
	return username, ""
}

func (s *Server) routeID(auth authContext) string {
	if auth.Region == "" {
		return s.ChannelID
	}
	return s.ChannelID + "#region=" + auth.Region
}

func (s *Server) handleConn(conn net.Conn) {
	if timeout := s.handshakeTimeout(); timeout > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
	}
	var first [1]byte
	if _, err := conn.Read(first[:]); err != nil {
		_ = conn.Close()
		return
	}
	if first[0] == 0x05 {
		s.socks5Handler(conn)
		return
	}
	s.httpHandler(conn, first[0])
}

func (s *Server) handshakeTimeout() time.Duration {
	if s.HandshakeTimeout > 0 {
		return s.HandshakeTimeout
	}
	return 10 * time.Second
}
