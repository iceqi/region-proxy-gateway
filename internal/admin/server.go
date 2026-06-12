package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/iceqi/region-proxy-gateway/internal/channel"
	"github.com/iceqi/region-proxy-gateway/internal/config"
	"github.com/iceqi/region-proxy-gateway/internal/connection"
	"github.com/iceqi/region-proxy-gateway/internal/node"
)

type Server struct {
	channels    *channel.Manager
	nodes       *node.Store
	connections *connection.Tracker
	configPath  string
	config      config.Config
	configMu    sync.Mutex
}

type Option func(*Server)

func WithConfig(path string, cfg config.Config) Option {
	return func(s *Server) {
		s.configPath = path
		s.config = cfg
	}
}

func NewServer(channels *channel.Manager, nodes *node.Store, connections *connection.Tracker, opts ...Option) *Server {
	server := &Server{
		channels:    channels,
		nodes:       nodes,
		connections: connections,
	}
	for _, opt := range opts {
		opt(server)
	}
	return server
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		s.writeHTML(w, http.StatusOK, indexHTML)
		return
	}

	if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/channels/") && strings.HasSuffix(r.URL.Path, "/switch") {
		s.handleSwitch(w, r)
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/api/channels" {
		s.handleSaveChannel(w, r)
		return
	}
	if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/channels/") {
		s.handleDeleteChannel(w, r)
		return
	}

	if r.Method != http.MethodGet {
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	switch r.URL.Path {
	case "/api/status":
		s.writeJSON(w, http.StatusOK, map[string]any{
			"ok":               true,
			"channel_count":    len(s.channelList()),
			"node_count":       len(s.nodes.List()),
			"connection_count": s.connectionCount(),
		})
	case "/api/channels":
		s.writeJSON(w, http.StatusOK, map[string]any{
			"channels": s.channelList(),
		})
	case "/api/connections":
		s.writeJSON(w, http.StatusOK, map[string]any{
			"connections": s.connectionList(),
		})
	case "/api/nodes":
		s.writeJSON(w, http.StatusOK, map[string]any{
			"nodes": s.nodeList(r.URL.Query().Get("region")),
		})
	default:
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func (s *Server) handleSwitch(w http.ResponseWriter, r *http.Request) {
	if s.channels == nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "channels unavailable"})
		return
	}
	channelID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/channels/"), "/switch")
	channelID = strings.Trim(channelID, "/")
	if channelID == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "channel id is required"})
		return
	}
	var body struct {
		NodeID string `json:"node_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if strings.TrimSpace(body.NodeID) == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node_id is required"})
		return
	}
	if err := s.channels.SwitchToNode(context.Background(), channelID, strings.TrimSpace(body.NodeID)); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	snapshot, _ := s.channels.Snapshot(channelID)
	s.writeJSON(w, http.StatusOK, map[string]any{"channel": snapshot})
}

func (s *Server) handleSaveChannel(w http.ResponseWriter, r *http.Request) {
	var ch config.Channel
	if err := json.NewDecoder(r.Body).Decode(&ch); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if err := ch.Validate(); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	cfg, err := s.updateConfig(func(cfg config.Config) (config.Config, error) {
		replaced := false
		for i := range cfg.Channels {
			if cfg.Channels[i].ID == ch.ID {
				cfg.Channels[i] = ch
				replaced = true
				break
			}
		}
		if !replaced {
			cfg.Channels = append(cfg.Channels, ch)
		}
		return cfg, nil
	})
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"config": cfg, "restart_required": true})
}

func (s *Server) handleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	channelID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/channels/"), "/")
	if channelID == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "channel id is required"})
		return
	}
	cfg, err := s.updateConfig(func(cfg config.Config) (config.Config, error) {
		channels := make([]config.Channel, 0, len(cfg.Channels))
		found := false
		for _, ch := range cfg.Channels {
			if ch.ID == channelID {
				found = true
				continue
			}
			channels = append(channels, ch)
		}
		if !found {
			return cfg, nil
		}
		cfg.Channels = channels
		return cfg, nil
	})
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"config": cfg, "restart_required": true})
}

func (s *Server) updateConfig(update func(config.Config) (config.Config, error)) (config.Config, error) {
	s.configMu.Lock()
	defer s.configMu.Unlock()

	if s.configPath == "" {
		return config.Config{}, fmt.Errorf("config persistence is not configured")
	}
	cfg := s.config
	if cfg.AdminPort == 0 {
		loaded, err := config.Load(s.configPath)
		if err != nil {
			return config.Config{}, err
		}
		cfg = loaded
	}
	updated, err := update(cfg)
	if err != nil {
		return config.Config{}, err
	}
	if err := config.Save(s.configPath, updated); err != nil {
		return config.Config{}, err
	}
	s.config = updated
	return updated, nil
}

func (s *Server) channelList() []channel.Snapshot {
	if s.channels == nil {
		return []channel.Snapshot{}
	}
	return s.channels.Snapshots()
}

func (s *Server) connectionCount() int {
	if s.connections == nil {
		return 0
	}
	return s.connections.ActiveCount()
}

func (s *Server) connectionList() []connection.Record {
	if s.connections == nil {
		return []connection.Record{}
	}
	return s.connections.List()
}

func (s *Server) nodeList(region string) []node.Node {
	if s.nodes == nil {
		return []node.Node{}
	}
	region = strings.ToLower(strings.TrimSpace(region))
	nodes := s.nodes.List()
	if region == "" {
		return nodes
	}
	filtered := make([]node.Node, 0, len(nodes))
	for _, n := range nodes {
		if n.Region == region {
			filtered = append(filtered, n)
		}
	}
	return filtered
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (s *Server) writeHTML(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
