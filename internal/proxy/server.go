package proxy

import (
	"context"
	"errors"
	"net"

	"github.com/iceqi/region-proxy-gateway/internal/connection"
	"github.com/iceqi/region-proxy-gateway/internal/session"
	"github.com/iceqi/region-proxy-gateway/internal/strategy"
)

type SessionProvider interface {
	GetOrCreate(ctx context.Context, strat strategy.Strategy) (session.Session, error)
}

type httpHandlerFunc func(conn net.Conn, firstByte byte)
type socks5HandlerFunc func(conn net.Conn)

type Server struct {
	ListenAddr           string
	ProxyPassword        string
	AllowedRegions       []string
	AllowedRotateMinutes []int

	sessions    SessionProvider
	connections *connection.Tracker

	httpHandler   httpHandlerFunc
	socks5Handler socks5HandlerFunc
}

func NewServer(listenAddr string, proxyPassword string, allowedRegions []string, allowedRotateMinutes []int, sessions SessionProvider, connections *connection.Tracker) *Server {
	server := &Server{
		ListenAddr:           listenAddr,
		ProxyPassword:        proxyPassword,
		AllowedRegions:       allowedRegions,
		AllowedRotateMinutes: allowedRotateMinutes,
		sessions:             sessions,
		connections:          connections,
	}
	server.httpHandler = server.handleHTTP
	server.socks5Handler = server.handleSOCKS5
	return server
}

func (s *Server) Serve(listener net.Listener) error {
	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			if opErr, ok := err.(*net.OpError); ok && !opErr.Temporary() {
				return nil
			}
			return err
		}

		go s.handleConn(conn)
	}
}

func (s *Server) authenticate(username, password string) (strategy.Strategy, error) {
	if !CheckPassword(password, s.ProxyPassword) {
		return strategy.Strategy{}, errors.New("invalid proxy credentials")
	}

	strat, err := strategy.Parse(username)
	if err != nil {
		return strategy.Strategy{}, errors.New("invalid proxy credentials")
	}
	if !containsString(s.AllowedRegions, strat.Region) {
		return strategy.Strategy{}, errors.New("invalid proxy credentials")
	}
	if !containsInt(s.AllowedRotateMinutes, strat.RotateMinutes) {
		return strategy.Strategy{}, errors.New("invalid proxy credentials")
	}

	return strat, nil
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func containsInt(items []int, want int) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func (s *Server) handleConn(conn net.Conn) {
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
