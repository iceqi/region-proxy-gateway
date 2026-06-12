package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/iceqi/region-proxy-gateway/internal/channel"
	"github.com/iceqi/region-proxy-gateway/internal/connection"
	"github.com/iceqi/region-proxy-gateway/internal/node"
)

type Server struct {
	channels    *channel.Manager
	nodes       *node.Store
	connections *connection.Tracker
}

func NewServer(channels *channel.Manager, nodes *node.Store, connections *connection.Tracker) *Server {
	return &Server{
		channels:    channels,
		nodes:       nodes,
		connections: connections,
	}
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
