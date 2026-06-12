package admin

import (
	"encoding/json"
	"net/http"

	"github.com/iceqi/region-proxy-gateway/internal/node"
	"github.com/iceqi/region-proxy-gateway/internal/session"
)

type Server struct {
	sessions *session.Manager
	nodes    *node.Store
}

func NewServer(sessions *session.Manager, nodes *node.Store) *Server {
	return &Server{
		sessions: sessions,
		nodes:    nodes,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	switch r.URL.Path {
	case "/api/status":
		s.writeJSON(w, http.StatusOK, map[string]any{
			"ok":              true,
			"active_sessions": s.sessions.ActiveCount(),
			"node_count":      len(s.nodes.List()),
		})
	case "/api/sessions":
		s.writeJSON(w, http.StatusOK, map[string]any{
			"sessions": s.sessions.List(),
		})
	case "/api/nodes":
		s.writeJSON(w, http.StatusOK, map[string]any{
			"nodes": s.nodes.List(),
		})
	default:
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, body any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
