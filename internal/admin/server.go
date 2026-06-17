package admin

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/channel"
	"github.com/iceqi/region-proxy-gateway/internal/config"
	"github.com/iceqi/region-proxy-gateway/internal/connection"
	"github.com/iceqi/region-proxy-gateway/internal/deeptest"
	"github.com/iceqi/region-proxy-gateway/internal/node"
	"github.com/iceqi/region-proxy-gateway/internal/storage"
)

type Server struct {
	channels              *channel.Manager
	nodes                 *node.Store
	connections           *connection.Tracker
	refreshNodes          func(context.Context) ([]node.Node, error)
	checkNode             func(context.Context, node.Node) node.Node
	restarter             func(context.Context) error
	reloadRuntime         func(context.Context) error
	configPath            string
	config                config.Config
	configMu              sync.Mutex
	proxyExtractMu        sync.Mutex
	proxyExtract          proxyExtractCache
	proxyExtractCursor    map[string]int
	proxyExtractValidator proxyExtractValidator
	storage               *storage.Store
	adminPath             string
	adminUser             string
	adminPass             string
}

type Option func(*Server)

type proxyExtractValidator func(context.Context, proxyExtractItem) error

func WithConfig(path string, cfg config.Config) Option {
	return func(s *Server) {
		s.configPath = path
		s.config = cfg
		s.adminPath = normalizeAdminPath(cfg.AdminPath)
		s.adminUser = cfg.AdminUsername
		s.adminPass = cfg.AdminPassword
	}
}

func WithAdminPath(path string) Option {
	return func(s *Server) {
		s.adminPath = normalizeAdminPath(path)
	}
}

func WithAdminAuth(username, password string) Option {
	return func(s *Server) {
		s.adminUser = username
		s.adminPass = password
	}
}

func WithNodeRefresher(refresh func(context.Context) ([]node.Node, error)) Option {
	return func(s *Server) {
		s.refreshNodes = refresh
	}
}

func WithNodeChecker(check func(context.Context, node.Node) node.Node) Option {
	return func(s *Server) {
		s.checkNode = check
	}
}

func WithStorage(store *storage.Store) Option {
	return func(s *Server) {
		s.storage = store
	}
}

func WithRestarter(restart func(context.Context) error) Option {
	return func(s *Server) {
		s.restarter = restart
	}
}

func WithRuntimeReloader(reload func(context.Context) error) Option {
	return func(s *Server) {
		s.reloadRuntime = reload
	}
}

func WithProxyExtractorValidator(validator proxyExtractValidator) Option {
	return func(s *Server) {
		s.proxyExtractValidator = validator
	}
}

func NewServer(channels *channel.Manager, nodes *node.Store, connections *connection.Tracker, opts ...Option) *Server {
	server := &Server{
		channels:    channels,
		nodes:       nodes,
		connections: connections,
		adminPath:   "/admin",
	}
	for _, opt := range opts {
		opt(server)
	}
	return server
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	adminPath := s.currentAdminPath()
	if r.URL.Path == adminPath {
		http.Redirect(w, r, adminPath+"/", http.StatusMovedPermanently)
		return
	}
	if r.URL.Path == adminPath+"/" {
		s.writeHTML(w, http.StatusOK, indexHTML)
		return
	}
	if !strings.HasPrefix(r.URL.Path, adminPath+"/") {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if !s.authorized(r) && !s.authorizedProxyExtractToken(r, adminPath) {
		s.writeUnauthorized(w)
		return
	}

	originalPath := r.URL.Path
	r.URL.Path = strings.TrimPrefix(r.URL.Path, adminPath)
	if r.URL.Path == "" {
		r.URL.Path = "/"
	}
	defer func() { r.URL.Path = originalPath }()

	if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/channels/") && strings.HasSuffix(r.URL.Path, "/switch") {
		s.handleSwitch(w, r)
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/api/channels" {
		s.handleSaveChannel(w, r)
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/api/nodes/refresh" {
		s.handleRefreshNodes(w, r)
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/api/nodes/probe-batch" {
		s.handleProbeNodes(w, r)
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/api/deep-tests" {
		s.handleEnqueueDeepTests(w, r)
		return
	}
	if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/nodes/") && strings.HasSuffix(r.URL.Path, "/probe") {
		s.handleProbeNode(w, r)
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/api/settings" {
		s.handleSaveSettings(w, r)
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/api/settings/proxy-extract-token" {
		s.handleGenerateProxyExtractToken(w, r)
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/api/system/restart" {
		s.handleRestart(w, r)
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
			"settings":         s.safeSettings(),
		})
	case "/api/settings":
		s.writeJSON(w, http.StatusOK, map[string]any{"settings": s.safeSettings()})
	case "/api/channels":
		s.writeJSON(w, http.StatusOK, map[string]any{
			"channels": s.channelViewList(),
		})
	case "/api/connections":
		s.writeJSON(w, http.StatusOK, map[string]any{
			"connections": s.connectionList(),
		})
	case "/api/nodes":
		s.writeJSON(w, http.StatusOK, map[string]any{
			"nodes": s.nodeViewList(r.URL.Query().Get("region")),
		})
	case "/api/proxies/extract":
		s.handleExtractProxies(w, r)
	case "/api/deep-tests/status":
		s.handleDeepTestStatus(w, r)
	default:
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func (s *Server) authorized(r *http.Request) bool {
	_, adminUser, adminPass := s.currentAdminAuth()
	if adminUser == "" && adminPass == "" {
		return true
	}
	username, password, ok := r.BasicAuth()
	if !ok {
		return false
	}
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(adminUser)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(password), []byte(adminPass)) == 1
	return userOK && passOK
}

func (s *Server) authorizedProxyExtractToken(r *http.Request, adminPath string) bool {
	if r.Method != http.MethodGet {
		return false
	}
	if strings.TrimPrefix(r.URL.Path, adminPath) != "/api/proxies/extract" {
		return false
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		return false
	}
	s.configMu.Lock()
	expected := s.config.ProxyExtractAPIToken
	s.configMu.Unlock()
	if expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

func (s *Server) currentAdminPath() string {
	adminPath, _, _ := s.currentAdminAuth()
	return adminPath
}

func (s *Server) currentAdminAuth() (string, string, string) {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	adminPath := s.adminPath
	if adminPath == "" {
		adminPath = "/admin"
	}
	return adminPath, s.adminUser, s.adminPass
}

func (s *Server) writeUnauthorized(w http.ResponseWriter) {
	s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
}

func normalizeAdminPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/admin"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		return "/admin"
	}
	return path
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
	nodeID := strings.TrimSpace(body.NodeID)
	if err := s.channels.SwitchToNode(context.Background(), channelID, nodeID); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	snapshot, _ := s.channels.Snapshot(channelID)
	if err := s.persistManualNode(r.Context(), snapshot, nodeID); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"channel":          snapshot,
		"channel_reloaded": true,
	})
}

func (s *Server) handleSaveChannel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OriginalID string `json:"original_id"`
		config.Channel
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	ch := body.Channel
	ch.Region = strings.ToLower(strings.TrimSpace(ch.Region))
	if err := ch.Validate(); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	originalID := strings.TrimSpace(body.OriginalID)
	if originalID == "" {
		originalID = ch.ID
	}
	cfg, err := s.updateConfig(func(cfg config.Config) (config.Config, error) {
		if s.storage != nil {
			if err := s.storage.SaveChannel(r.Context(), originalID, ch); err != nil {
				return cfg, err
			}
			return cfg, nil
		}
		cfg.Channels = saveChannelInConfig(cfg.Channels, originalID, ch)
		return cfg, nil
	})
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	reloadOK, reloadError := s.reloadRuntimeState(r.Context())
	s.writeJSON(w, http.StatusOK, map[string]any{
		"config":            cfg,
		"runtime_reloaded":  reloadOK,
		"runtime_error":     reloadError,
		"restart_required":  false,
		"restart_scheduled": false,
	})
}

func (s *Server) handleRefreshNodes(w http.ResponseWriter, r *http.Request) {
	if s.refreshNodes == nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node refresher is not configured"})
		return
	}
	nodes, err := s.refreshNodes(r.Context())
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if s.nodes != nil {
		s.nodes.Replace(nodes)
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"nodes": s.nodeViewsFrom(nodes, "")})
}

func (s *Server) handleProbeNode(w http.ResponseWriter, r *http.Request) {
	if s.nodes == nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node store is not configured"})
		return
	}
	if s.checkNode == nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node checker is not configured"})
		return
	}
	nodeID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/nodes/"), "/probe")
	nodeID = strings.Trim(nodeID, "/")
	if nodeID == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node id is required"})
		return
	}
	var checked node.Node
	ok := s.nodes.Update(nodeID, func(n node.Node) node.Node {
		checked = s.checkNode(r.Context(), n)
		return checked
	})
	if !ok {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "node not found"})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"node": checked})
}

func (s *Server) handleProbeNodes(w http.ResponseWriter, r *http.Request) {
	if s.nodes == nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node store is not configured"})
		return
	}
	if s.checkNode == nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node checker is not configured"})
		return
	}
	var body struct {
		NodeIDs []string `json:"node_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	nodeIDs := cleanNodeIDs(body.NodeIDs, 120)
	if len(nodeIDs) == 0 {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node_ids is required"})
		return
	}

	type result struct {
		id   string
		node node.Node
		ok   bool
	}
	nodeByID := make(map[string]node.Node, len(nodeIDs))
	for _, n := range s.nodes.List() {
		nodeByID[n.ID] = n
	}
	const workers = 8
	jobs := make(chan string)
	results := make(chan result, len(nodeIDs))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for nodeID := range jobs {
				n, ok := nodeByID[nodeID]
				if !ok {
					results <- result{id: nodeID}
					continue
				}
				checked := s.checkNode(r.Context(), n)
				s.nodes.Update(nodeID, func(node.Node) node.Node { return checked })
				results <- result{id: nodeID, node: checked, ok: true}
			}
		}()
	}
	for _, nodeID := range nodeIDs {
		jobs <- nodeID
	}
	close(jobs)
	wg.Wait()
	close(results)

	checked := make([]node.Node, 0, len(nodeIDs))
	missing := make([]string, 0)
	for result := range results {
		if !result.ok {
			missing = append(missing, result.id)
			continue
		}
		checked = append(checked, result.node)
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"count": len(checked), "nodes": checked, "missing": missing})
}

func (s *Server) handleEnqueueDeepTests(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "storage is not configured"})
		return
	}
	var body struct {
		NodeIDs []string `json:"node_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	nodeIDs := cleanNodeIDsKeepingDuplicates(body.NodeIDs, 500)
	if len(nodeIDs) == 0 {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node_ids is required"})
		return
	}
	summary, err := s.storage.EnqueueDeepTestJobs(r.Context(), nodeIDs)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	stats, err := s.storage.DeepTestQueueStats(r.Context())
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"summary": summary, "stats": stats})
}

func (s *Server) handleDeepTestStatus(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "storage is not configured"})
		return
	}
	stats, err := s.storage.DeepTestQueueStats(r.Context())
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"stats": stats})
}

func cleanNodeIDs(ids []string, limit int) []string {
	seen := make(map[string]struct{}, len(ids))
	cleaned := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		cleaned = append(cleaned, id)
		if limit > 0 && len(cleaned) >= limit {
			break
		}
	}
	return cleaned
}

func cleanNodeIDsKeepingDuplicates(ids []string, limit int) []string {
	cleaned := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		cleaned = append(cleaned, id)
		if limit > 0 && len(cleaned) >= limit {
			break
		}
	}
	return cleaned
}

func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NodeRefreshInterval  string `json:"node_refresh_interval"`
		ProxyExtractCacheTTL string `json:"proxy_extract_cache_ttl"`
		AdminPath            string `json:"admin_path"`
		AdminUsername        string `json:"admin_username"`
		AdminPassword        string `json:"admin_password"`
		ProxyUsername        string `json:"proxy_username"`
		ProxyPassword        string `json:"proxy_password"`
		ProxyExtractAPIToken string `json:"proxy_extract_api_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	interval := strings.TrimSpace(body.NodeRefreshInterval)
	if interval == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node_refresh_interval is required"})
		return
	}
	cfg, err := s.updateConfig(func(cfg config.Config) (config.Config, error) {
		cfg.NodeRefreshInterval = interval
		if strings.TrimSpace(body.ProxyExtractCacheTTL) != "" {
			cfg.ProxyExtractCacheTTL = strings.TrimSpace(body.ProxyExtractCacheTTL)
		}
		if strings.TrimSpace(body.AdminPath) != "" {
			adminPath, err := normalizeEditableAdminPath(body.AdminPath)
			if err != nil {
				return cfg, err
			}
			cfg.AdminPath = adminPath
		}
		if strings.TrimSpace(body.AdminUsername) != "" {
			cfg.AdminUsername = strings.TrimSpace(body.AdminUsername)
		}
		if strings.TrimSpace(body.AdminPassword) != "" {
			cfg.AdminPassword = strings.TrimSpace(body.AdminPassword)
		}
		if strings.TrimSpace(body.ProxyUsername) != "" {
			cfg.ProxyUsername = strings.TrimSpace(body.ProxyUsername)
		}
		if strings.TrimSpace(body.ProxyPassword) != "" {
			cfg.ProxyPassword = strings.TrimSpace(body.ProxyPassword)
		}
		if strings.TrimSpace(body.ProxyExtractAPIToken) != "" {
			cfg.ProxyExtractAPIToken = strings.TrimSpace(body.ProxyExtractAPIToken)
		}
		if _, err := config.ParseNodeRefreshInterval(interval); err != nil {
			return cfg, err
		}
		if _, err := config.ParseProxyExtractCacheTTL(cfg.ProxyExtractCacheTTL); err != nil {
			return cfg, err
		}
		if err := cfg.Validate(); err != nil {
			return cfg, err
		}
		return cfg, nil
	})
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.applyAuthConfig(cfg)
	reloadOK, reloadError := s.reloadRuntimeState(r.Context())
	s.writeJSON(w, http.StatusOK, map[string]any{
		"settings":         s.safeSettings(),
		"runtime_reloaded": reloadOK,
		"runtime_error":    reloadError,
		"restart_required": false,
	})
}

func (s *Server) handleGenerateProxyExtractToken(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.updateConfig(func(cfg config.Config) (config.Config, error) {
		cfg.ProxyExtractAPIToken = randomToken(32)
		return cfg, nil
	})
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"settings": s.safeSettingsWith(cfg),
		"token":    cfg.ProxyExtractAPIToken,
	})
}

type proxyExtractCache struct {
	key       string
	expiresAt time.Time
	items     []proxyExtractItem
}

type proxyExtractItem struct {
	ChannelID     string `json:"channel_id"`
	Region        string `json:"region"`
	CurrentExitIP string `json:"current_exit_ip"`
	HTTP          string `json:"http"`
	SOCKS5        string `json:"socks5"`
	NoScheme      string `json:"no_scheme"`
	ValidatedAt   string `json:"validated_at"`
	Cached        bool   `json:"cached"`
	listenHost    string
	listenPort    int
	username      string
	password      string
}

func (s *Server) handleExtractProxies(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	format := strings.ToLower(strings.TrimSpace(query.Get("format")))
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "text" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "format must be json or text"})
		return
	}
	scheme := strings.ToLower(strings.TrimSpace(query.Get("scheme")))
	if scheme == "" {
		scheme = "http"
	}
	if scheme != "http" && scheme != "socks5" && scheme != "no-scheme" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scheme must be http, socks5, or no-scheme"})
		return
	}
	count := 1
	if raw := strings.TrimSpace(query.Get("count")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "count must be >= 1"})
			return
		}
		count = parsed
	}
	region := strings.ToLower(strings.TrimSpace(query.Get("region")))
	rotate := parseBoolQuery(query.Get("rotate"))

	items, cached, err := s.extractProxies(r.Context(), proxyExtractRequest{
		region: region,
		scheme: scheme,
		count:  count,
		host:   requestHost(r),
		rotate: rotate,
	})
	if err != nil {
		s.writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	for i := range items {
		items[i].Cached = cached
	}
	if format == "text" {
		lines := make([]string, 0, len(items))
		for _, item := range items {
			lines = append(lines, item.proxyForScheme(scheme))
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Join(lines, "\n") + "\n"))
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"proxies": items,
		"cached":  cached,
		"ttl":     s.proxyExtractCacheTTL().String(),
	})
}

type proxyExtractRequest struct {
	region string
	scheme string
	count  int
	host   string
	rotate bool
}

func (r proxyExtractRequest) cacheKey() string {
	return r.region + "|" + r.scheme + "|" + strconv.Itoa(r.count) + "|" + r.host
}

func (s *Server) extractProxies(ctx context.Context, req proxyExtractRequest) ([]proxyExtractItem, bool, error) {
	ttl := s.proxyExtractCacheTTL()
	key := req.cacheKey()
	now := time.Now()
	if ttl > 0 && !req.rotate {
		s.proxyExtractMu.Lock()
		if s.proxyExtract.key == key && now.Before(s.proxyExtract.expiresAt) && len(s.proxyExtract.items) >= req.count {
			items := append([]proxyExtractItem(nil), s.proxyExtract.items[:req.count]...)
			s.proxyExtractMu.Unlock()
			return items, true, nil
		}
		s.proxyExtractMu.Unlock()
	}
	if req.rotate {
		s.rotateExtractChannels(ctx, req)
	} else {
		s.rotateStaleExtractChannels(ctx, req)
	}

	candidates := s.extractCandidates(req)
	items := make([]proxyExtractItem, 0, req.count)
	for _, item := range candidates {
		validateCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		err := s.validateExtractedProxy(validateCtx, item)
		cancel()
		if err != nil {
			continue
		}
		item.ValidatedAt = time.Now().Format(time.RFC3339)
		items = append(items, item)
		if len(items) >= req.count {
			break
		}
	}
	if len(items) == 0 {
		return nil, false, fmt.Errorf("no validated proxies available")
	}
	if ttl > 0 && !req.rotate {
		s.proxyExtractMu.Lock()
		s.proxyExtract = proxyExtractCache{key: key, expiresAt: time.Now().Add(ttl), items: append([]proxyExtractItem(nil), items...)}
		s.proxyExtractMu.Unlock()
	}
	return items, false, nil
}

func (s *Server) rotateExtractChannels(ctx context.Context, req proxyExtractRequest) map[string]struct{} {
	rotated := map[string]struct{}{}
	if s.channels == nil {
		return rotated
	}
	for _, snapshot := range s.channelList() {
		if !snapshot.Enabled {
			continue
		}
		if req.region != "" && req.region != "*" && snapshot.Region != req.region {
			continue
		}
		rotateCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err := s.channels.RotateNow(rotateCtx, snapshot.ID)
		cancel()
		if err == nil {
			rotated[snapshot.ID] = struct{}{}
		}
	}
	return rotated
}

func (s *Server) rotateStaleExtractChannels(ctx context.Context, req proxyExtractRequest) map[string]struct{} {
	rotated := map[string]struct{}{}
	if s.channels == nil {
		return rotated
	}
	for _, snapshot := range s.channelList() {
		if !snapshot.Enabled || !snapshot.TunnelStatus.Ready {
			continue
		}
		if req.region != "" && req.region != "*" && snapshot.Region != req.region {
			continue
		}
		if s.freshProxyNode(snapshot.CurrentNode) {
			continue
		}
		rotateCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err := s.channels.RotateNow(rotateCtx, snapshot.ID)
		cancel()
		if err == nil {
			rotated[snapshot.ID] = struct{}{}
		}
	}
	return rotated
}

func parseBoolQuery(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func (s *Server) extractCandidates(req proxyExtractRequest) []proxyExtractItem {
	s.configMu.Lock()
	username := s.config.ProxyUsername
	password := s.config.ProxyPassword
	s.configMu.Unlock()

	entry, ok := s.proxyEntrySnapshot(req)
	if !ok {
		return nil
	}
	nodes := s.freshProxyNodes(req.region)
	if req.rotate {
		nodes = s.rotateProxyNodeOrder(req, nodes)
	}
	items := make([]proxyExtractItem, 0, len(nodes))
	for _, n := range nodes {
		host := proxyPublicHost(entry.ListenHost, req.host)
		routeUsername := proxyRegionUsername(username, n.Region)
		noScheme := proxyAuthHost(routeUsername, password, host, entry.ListenPort)
		items = append(items, proxyExtractItem{
			ChannelID:     entry.ID,
			Region:        n.Region,
			CurrentExitIP: nodeExitIP(n),
			HTTP:          "http://" + noScheme,
			SOCKS5:        "socks5://" + noScheme,
			NoScheme:      noScheme,
			listenHost:    entry.ListenHost,
			listenPort:    entry.ListenPort,
			username:      routeUsername,
			password:      password,
		})
	}
	return items
}

func (s *Server) proxyEntrySnapshot(req proxyExtractRequest) (channel.Snapshot, bool) {
	snapshots := s.channelList()
	for _, snapshot := range snapshots {
		if snapshot.Enabled && snapshot.TunnelStatus.Ready {
			return snapshot, true
		}
	}
	for _, snapshot := range snapshots {
		if snapshot.Enabled {
			return snapshot, true
		}
	}
	return channel.Snapshot{}, false
}

func (s *Server) freshProxyNodes(region string) []node.Node {
	if s.nodes == nil {
		return nil
	}
	nodes := s.nodes.List()
	fresh := make([]node.Node, 0, len(nodes))
	for _, n := range nodes {
		if region != "" && region != "*" && n.Region != region {
			continue
		}
		if s.freshProxyNode(n) {
			fresh = append(fresh, n)
		}
	}
	return fresh
}

func proxyRegionUsername(username string, region string) string {
	region = strings.ToLower(strings.TrimSpace(region))
	if region == "" || region == "*" {
		return username
	}
	return username + "+" + region
}

func nodeExitIP(n node.Node) string {
	if n.IP != "" {
		return n.IP
	}
	return n.Hostname
}

func (s *Server) rotateProxyNodeOrder(req proxyExtractRequest, nodes []node.Node) []node.Node {
	if len(nodes) <= 1 {
		return nodes
	}
	key := req.region
	if key == "" {
		key = "*"
	}
	s.proxyExtractMu.Lock()
	if s.proxyExtractCursor == nil {
		s.proxyExtractCursor = map[string]int{}
	}
	start := s.proxyExtractCursor[key] % len(nodes)
	s.proxyExtractCursor[key] = (start + 1) % len(nodes)
	s.proxyExtractMu.Unlock()
	ordered := make([]node.Node, 0, len(nodes))
	ordered = append(ordered, nodes[start:]...)
	ordered = append(ordered, nodes[:start]...)
	return ordered
}

func (s *Server) freshProxyNode(n node.Node) bool {
	if n.ID == "" || n.LastTestedAt.IsZero() {
		return false
	}
	s.configMu.Lock()
	value := s.config.NodeRefreshInterval
	s.configMu.Unlock()
	ttl, err := config.ParseNodeRefreshInterval(value)
	if err != nil {
		ttl = 10 * time.Minute
	}
	return time.Since(n.LastTestedAt) <= ttl
}

func (s *Server) validateExtractedProxy(ctx context.Context, item proxyExtractItem) error {
	if s.proxyExtractValidator != nil {
		return s.proxyExtractValidator(ctx, item)
	}
	address := net.JoinHostPort(proxyValidationHost(item.listenHost), strconv.Itoa(item.listenPort))
	dialer := net.Dialer{Timeout: 8 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	auth := base64.StdEncoding.EncodeToString([]byte(item.username + ":" + item.password))
	if _, err := fmt.Fprintf(conn, "CONNECT 1.1.1.1:443 HTTP/1.1\r\nHost: 1.1.1.1:443\r\nProxy-Authorization: Basic %s\r\n\r\n", auth); err != nil {
		return err
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return err
	}
	if !strings.Contains(line, " 200 ") {
		return fmt.Errorf("proxy validation returned %s", strings.TrimSpace(line))
	}
	return nil
}

func (s *Server) proxyExtractCacheTTL() time.Duration {
	s.configMu.Lock()
	value := s.config.ProxyExtractCacheTTL
	s.configMu.Unlock()
	ttl, err := config.ParseProxyExtractCacheTTL(value)
	if err != nil {
		return 30 * time.Second
	}
	return ttl
}

func (p proxyExtractItem) proxyForScheme(scheme string) string {
	switch scheme {
	case "socks5":
		return p.SOCKS5
	case "no-scheme":
		return p.NoScheme
	default:
		return p.HTTP
	}
}

func requestHost(r *http.Request) string {
	host := r.Host
	if host == "" {
		return ""
	}
	if strings.Contains(host, ":") {
		if h, _, err := net.SplitHostPort(host); err == nil {
			return h
		}
	}
	return host
}

func proxyPublicHost(listenHost string, requestHost string) string {
	host := strings.TrimSpace(listenHost)
	if host == "" || host == "0.0.0.0" || host == "127.0.0.1" || host == "localhost" || host == "::" {
		if requestHost != "" {
			return requestHost
		}
		return host
	}
	return host
}

func proxyValidationHost(listenHost string) string {
	host := strings.TrimSpace(listenHost)
	if host == "" || host == "0.0.0.0" || host == "::" {
		return "127.0.0.1"
	}
	return host
}

func proxyAuthHost(username string, password string, host string, port int) string {
	auth := ""
	if username != "" || password != "" {
		auth = url.PathEscape(username) + ":" + url.PathEscape(password) + "@"
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("%s%s:%d", auth, host, port)
}

func (s *Server) handleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	channelID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/channels/"), "/")
	if channelID == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "channel id is required"})
		return
	}
	cfg, err := s.updateConfig(func(cfg config.Config) (config.Config, error) {
		if s.storage != nil {
			if err := s.storage.DeleteChannel(r.Context(), channelID); err != nil {
				return cfg, err
			}
			return cfg, nil
		}
		cfg.Channels = deleteChannelFromConfig(cfg.Channels, channelID)
		return cfg, nil
	})
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	reloadOK, reloadError := s.reloadRuntimeState(r.Context())
	s.writeJSON(w, http.StatusOK, map[string]any{
		"config":            cfg,
		"runtime_reloaded":  reloadOK,
		"runtime_error":     reloadError,
		"restart_required":  false,
		"restart_scheduled": false,
	})
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if s.restarter == nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "service restart is not configured"})
		return
	}
	if err := s.restarter(r.Context()); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) scheduleRestart() (bool, string) {
	if s.restarter == nil {
		return false, ""
	}
	if err := s.restarter(context.Background()); err != nil {
		return false, err.Error()
	}
	return true, ""
}

func (s *Server) reloadRuntimeState(ctx context.Context) (bool, string) {
	if s.reloadRuntime == nil {
		return false, ""
	}
	if err := s.reloadRuntime(ctx); err != nil {
		return false, err.Error()
	}
	return true, ""
}

func (s *Server) persistManualNode(ctx context.Context, snapshot channel.Snapshot, nodeID string) error {
	ch := config.Channel{
		ID:            snapshot.ID,
		ListenHost:    snapshot.ListenHost,
		ListenPort:    snapshot.ListenPort,
		Region:        snapshot.Region,
		RotateMinutes: snapshot.RotateMinutes,
		RotateOnDial:  snapshot.RotateOnDial,
		SelectionMode: config.SelectionManual,
		ManualNodeID:  nodeID,
		Enabled:       snapshot.Enabled,
	}
	if s.storage != nil {
		return s.storage.SaveChannel(ctx, snapshot.ID, ch)
	}
	if s.configPath == "" {
		return nil
	}
	_, err := s.updateConfig(func(cfg config.Config) (config.Config, error) {
		cfg.Channels = saveChannelInConfig(cfg.Channels, snapshot.ID, ch)
		return cfg, nil
	})
	return err
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

type channelView struct {
	channel.Snapshot
	ProxyAuthHTTP   string `json:"proxy_auth_http"`
	ProxyAuthSOCKS5 string `json:"proxy_auth_socks5"`
}

func (s *Server) channelViewList() []channelView {
	s.configMu.Lock()
	proxyUser := s.config.ProxyUsername
	proxyPass := s.config.ProxyPassword
	configured := make([]config.Channel, len(s.config.Channels))
	copy(configured, s.config.Channels)
	s.configMu.Unlock()
	if s.storage != nil {
		channels, err := s.storage.ListChannels(context.Background())
		if err == nil {
			configured = channels
		}
	}

	runtimeByID := make(map[string]channel.Snapshot)
	for _, snapshot := range s.channelList() {
		runtimeByID[snapshot.ID] = snapshot
	}

	views := make([]channelView, 0, len(configured))
	for _, ch := range configured {
		ch.Region = strings.ToLower(strings.TrimSpace(ch.Region))
		snapshot, ok := runtimeByID[ch.ID]
		if !ok {
			snapshot = snapshotFromConfig(ch)
		} else {
			snapshot.ListenHost = ch.ListenHost
			snapshot.ListenPort = ch.ListenPort
			snapshot.Region = ch.Region
			snapshot.RotateMinutes = ch.RotateMinutes
			snapshot.RotateOnDial = ch.RotateOnDial
			snapshot.SelectionMode = ch.SelectionMode
			snapshot.ManualNodeID = ch.ManualNodeID
			snapshot.Enabled = ch.Enabled
			snapshot.ProxyURLHTTP = fmt.Sprintf("http://%s:%d", ch.ListenHost, ch.ListenPort)
			snapshot.ProxyURLSOCKS5 = fmt.Sprintf("socks5://%s:%d", ch.ListenHost, ch.ListenPort)
		}
		view := channelView{Snapshot: snapshot}
		if proxyUser != "" || proxyPass != "" {
			routeUser := proxyRegionUsername(proxyUser, snapshot.Region)
			view.ProxyAuthHTTP = fmt.Sprintf("http://%s:%s@%s:%d", routeUser, proxyPass, snapshot.ListenHost, snapshot.ListenPort)
			view.ProxyAuthSOCKS5 = fmt.Sprintf("socks5://%s:%s@%s:%d", routeUser, proxyPass, snapshot.ListenHost, snapshot.ListenPort)
		}
		views = append(views, view)
	}
	return views
}

func saveChannelInConfig(channels []config.Channel, originalID string, ch config.Channel) []config.Channel {
	replaced := false
	for i := range channels {
		if channels[i].ID == originalID {
			channels[i] = ch
			replaced = true
			break
		}
	}
	if !replaced {
		channels = append(channels, ch)
	}
	return channels
}

func deleteChannelFromConfig(channels []config.Channel, channelID string) []config.Channel {
	filtered := make([]config.Channel, 0, len(channels))
	for _, ch := range channels {
		if ch.ID != channelID {
			filtered = append(filtered, ch)
		}
	}
	return filtered
}

func snapshotFromConfig(ch config.Channel) channel.Snapshot {
	return channel.Snapshot{
		ID:             ch.ID,
		ListenHost:     ch.ListenHost,
		ListenPort:     ch.ListenPort,
		Region:         ch.Region,
		RotateMinutes:  ch.RotateMinutes,
		RotateOnDial:   ch.RotateOnDial,
		SelectionMode:  ch.SelectionMode,
		ManualNodeID:   ch.ManualNodeID,
		Enabled:        ch.Enabled,
		LastError:      "需要重启服务后生效",
		ProxyURLHTTP:   fmt.Sprintf("http://%s:%d", ch.ListenHost, ch.ListenPort),
		ProxyURLSOCKS5: fmt.Sprintf("socks5://%s:%d", ch.ListenHost, ch.ListenPort),
	}
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

type nodeView struct {
	ID           string        `json:"id"`
	Region       string        `json:"region"`
	Country      string        `json:"country"`
	IP           string        `json:"ip"`
	Hostname     string        `json:"hostname"`
	Port         int           `json:"port"`
	Proto        string        `json:"proto"`
	LatencyMS    int           `json:"latency_ms"`
	Speed        int64         `json:"speed"`
	Available    bool          `json:"available"`
	LastTestedAt time.Time     `json:"last_tested_at"`
	FailReason   string        `json:"fail_reason"`
	Owner        string        `json:"owner"`
	ASN          string        `json:"asn"`
	ASName       string        `json:"as_name"`
	Location     string        `json:"location"`
	IPType       string        `json:"ip_type"`
	Quality      string        `json:"quality"`
	PurityScore  int           `json:"purity_score"`
	ProbeStatus  string        `json:"probe_status"`
	ProbeMessage string        `json:"probe_message"`
	ProbedAt     time.Time     `json:"probed_at"`
	DeepTest     *deepTestView `json:"deep_test,omitempty"`
}

type deepTestView struct {
	Status      string    `json:"status"`
	ExitIP      string    `json:"exit_ip"`
	ExitCountry string    `json:"exit_country"`
	ConnectMS   int       `json:"connect_ms"`
	TestedAt    time.Time `json:"tested_at"`
	FailReason  string    `json:"fail_reason"`
}

func (s *Server) nodeViewList(region string) []nodeView {
	return s.nodeViewsFrom(s.nodeList(region), "")
}

func (s *Server) nodeViewsFrom(nodes []node.Node, region string) []nodeView {
	region = strings.ToLower(strings.TrimSpace(region))
	deepResults := map[string]deeptest.Result{}
	if s.storage != nil {
		if results, err := s.storage.DeepTestResults(context.Background()); err == nil {
			deepResults = results
		}
	}
	views := make([]nodeView, 0, len(nodes))
	for _, n := range nodes {
		if region != "" && n.Region != region {
			continue
		}
		view := compactNode(n)
		if result, ok := deepResults[n.ID]; ok {
			view.DeepTest = &deepTestView{
				Status:      result.Status,
				ExitIP:      result.ExitIP,
				ExitCountry: result.ExitCountry,
				ConnectMS:   result.ConnectMS,
				TestedAt:    result.TestedAt,
				FailReason:  result.FailReason,
			}
		}
		views = append(views, view)
	}
	return views
}

func compactNode(n node.Node) nodeView {
	return nodeView{
		ID:           n.ID,
		Region:       n.Region,
		Country:      n.Country,
		IP:           n.IP,
		Hostname:     n.Hostname,
		Port:         n.Port,
		Proto:        n.Proto,
		LatencyMS:    n.LatencyMS,
		Speed:        n.Speed,
		Available:    n.Available,
		LastTestedAt: n.LastTestedAt,
		FailReason:   n.FailReason,
		Owner:        n.Owner,
		ASN:          n.ASN,
		ASName:       n.ASName,
		Location:     n.Location,
		IPType:       n.IPType,
		Quality:      n.Quality,
		PurityScore:  n.PurityScore,
		ProbeStatus:  n.ProbeStatus,
		ProbeMessage: n.ProbeMessage,
		ProbedAt:     n.ProbedAt,
	}
}

type settingsView struct {
	NodeRefreshInterval  string `json:"node_refresh_interval"`
	ProxyExtractCacheTTL string `json:"proxy_extract_cache_ttl"`
	ProxyExtractAPIToken string `json:"proxy_extract_api_token"`
	AdminPath            string `json:"admin_path"`
	AdminUsername        string `json:"admin_username"`
	ProxyUsername        string `json:"proxy_username"`
	AdminPasswordSet     bool   `json:"admin_password_set"`
	ProxyPasswordSet     bool   `json:"proxy_password_set"`
}

func (s *Server) safeSettings() settingsView {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	return settingsView{
		NodeRefreshInterval:  s.config.NodeRefreshInterval,
		ProxyExtractCacheTTL: s.config.ProxyExtractCacheTTL,
		ProxyExtractAPIToken: s.config.ProxyExtractAPIToken,
		AdminPath:            s.config.AdminPath,
		AdminUsername:        s.config.AdminUsername,
		ProxyUsername:        s.config.ProxyUsername,
		AdminPasswordSet:     s.config.AdminPassword != "",
		ProxyPasswordSet:     s.config.ProxyPassword != "",
	}
}

func (s *Server) safeSettingsWith(cfg config.Config) settingsView {
	return settingsView{
		NodeRefreshInterval:  cfg.NodeRefreshInterval,
		ProxyExtractCacheTTL: cfg.ProxyExtractCacheTTL,
		ProxyExtractAPIToken: cfg.ProxyExtractAPIToken,
		AdminPath:            cfg.AdminPath,
		AdminUsername:        cfg.AdminUsername,
		ProxyUsername:        cfg.ProxyUsername,
		AdminPasswordSet:     cfg.AdminPassword != "",
		ProxyPasswordSet:     cfg.ProxyPassword != "",
	}
}

func randomToken(length int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var builder strings.Builder
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			builder.WriteByte('x')
			continue
		}
		builder.WriteByte(alphabet[n.Int64()])
	}
	return builder.String()
}

func (s *Server) applyAuthConfig(cfg config.Config) {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	s.adminPath = normalizeAdminPath(cfg.AdminPath)
	s.adminUser = cfg.AdminUsername
	s.adminPass = cfg.AdminPassword
}

func normalizeEditableAdminPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("admin path is required")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		path = "/"
	}
	return path, nil
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
