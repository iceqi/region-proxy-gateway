package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/admin"
	"github.com/iceqi/region-proxy-gateway/internal/channel"
	"github.com/iceqi/region-proxy-gateway/internal/config"
	"github.com/iceqi/region-proxy-gateway/internal/connection"
	"github.com/iceqi/region-proxy-gateway/internal/deeptest"
	"github.com/iceqi/region-proxy-gateway/internal/ipinfo"
	"github.com/iceqi/region-proxy-gateway/internal/node"
	"github.com/iceqi/region-proxy-gateway/internal/nodecheck"
	"github.com/iceqi/region-proxy-gateway/internal/proxy"
	"github.com/iceqi/region-proxy-gateway/internal/storage"
	"github.com/iceqi/region-proxy-gateway/internal/tunnel"
	"github.com/iceqi/region-proxy-gateway/internal/vpngate"
)

var version = "dev"

type services struct {
	admin          *admin.Server
	nodes          *node.Store
	channels       *channel.Manager
	tracker        *connection.Tracker
	storage        *storage.Store
	proxyRuntime   *proxyRuntime
	gatewayRuntime *proxyRuntime
	configPath     string
}

type nodeScanState struct {
	mu        sync.Mutex
	running   bool
	total     int
	success   int
	failed    int
	startedAt time.Time
	endedAt   time.Time
	lastError string
}

func (s *nodeScanState) start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = true
	s.total = 0
	s.success = 0
	s.failed = 0
	s.startedAt = time.Now()
	s.endedAt = time.Time{}
	s.lastError = ""
}

func (s *nodeScanState) progress(total int, success int, failed int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.total = total
	s.success = success
	s.failed = failed
}

func (s *nodeScanState) finish(total int, success int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	s.total = total
	s.success = success
	if total >= success {
		s.failed = total - success
	}
	s.endedAt = time.Now()
	if err != nil {
		s.lastError = err.Error()
	}
}

func (s *nodeScanState) snapshot() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{"running": s.running, "total": s.total, "success": s.success, "failed": s.failed, "started_at": s.startedAt, "ended_at": s.endedAt, "last_error": s.lastError}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version)
		return
	}

	cfgPath := filepath.Join("data", "config.json")
	cfg, err := config.LoadOrCreate(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	services, err := buildServices(ctx, cfg, cfgPath)
	if err != nil {
		log.Fatalf("build services: %v", err)
	}
	defer services.channels.Stop(ctx)
	defer services.storage.Close()

	adminAddr := fmt.Sprintf("%s:%d", cfg.AdminHost, cfg.AdminPort)
	log.Printf("admin listening on http://%s", adminAddr)
	adminServer := &http.Server{Addr: adminAddr, Handler: services.admin}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = adminServer.Shutdown(shutdownCtx)
		if services.proxyRuntime != nil {
			_ = services.proxyRuntime.Stop(shutdownCtx)
		}
		if services.gatewayRuntime != nil {
			_ = services.gatewayRuntime.Stop(shutdownCtx)
		}
	}()
	if err := adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func buildServices(ctx context.Context, cfg config.Config, cfgPath string) (services, error) {
	if err := cfg.Validate(); err != nil {
		return services{}, err
	}
	refreshInterval, err := config.ParseNodeRefreshInterval(cfg.NodeRefreshInterval)
	if err != nil {
		return services{}, err
	}

	database, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		return services{}, err
	}
	if err := database.MigrateChannels(ctx, cfg.Channels); err != nil {
		_ = database.Close()
		return services{}, err
	}
	if cfgPath != "" && len(cfg.Channels) > 0 {
		cfg.Channels = nil
		if err := config.Save(cfgPath, cfg); err != nil {
			_ = database.Close()
			return services{}, err
		}
	}
	channels, err := database.ListChannels(ctx)
	if err != nil {
		_ = database.Close()
		return services{}, err
	}

	nodes := node.NewStore()
	loadedNodes, err := database.ListNodes(ctx)
	if err != nil {
		_ = database.Close()
		return services{}, err
	}
	loadedNodes = freshNodes(loadedNodes, refreshInterval, time.Now())
	nodes.Replace(loadedNodes)
	lastNodeRefresh := latestNodeTestedAt(loadedNodes)
	refreshMu := sync.Mutex{}
	scanState := &nodeScanState{}
	forceRefreshNodes := func(ctx context.Context) ([]node.Node, error) {
		scanState.start()
		loaded, err := loadNodesWithProgress(ctx, cfg, scanState.progress)
		if err != nil {
			scanState.finish(0, 0, err)
			return nil, err
		}
		scanState.finish(len(loaded), len(loaded), nil)
		nodes.Replace(loaded)
		lastNodeRefresh = latestNodeTestedAt(loaded)
		if err := database.ReplaceNodes(ctx, loaded); err != nil {
			log.Printf("cache refreshed nodes failed: %v", err)
		}
		return loaded, nil
	}

	tracker := connection.NewTracker()
	checker := nodecheck.Checker{Timeout: 3 * time.Second}
	refreshNodes := func(ctx context.Context) error {
		refreshMu.Lock()
		defer refreshMu.Unlock()
		if len(nodes.List()) > 0 && !lastNodeRefresh.IsZero() && time.Since(lastNodeRefresh) < refreshInterval {
			return nil
		}
		_, err := forceRefreshNodes(ctx)
		return err
	}
	managerChannels := append(append([]config.Channel(nil), channels...), gatewayChannels(cfg)...)
	manager := channel.NewManager(channel.Config{
		Channels:      managerChannels,
		Nodes:         nodes,
		TunnelFactory: tunnelFactory(cfg),
		NodeChecker:   checker.Check,
		RefreshNodes:  refreshNodes,
		History:       channelHistoryAdapter{store: database},
		DataDir:       cfg.DataDir,
		OpenVPNCmd:    cfg.OpenVPNCommand,
	})
	if err := manager.Start(ctx); err != nil {
		return services{}, err
	}
	go func() {
		if len(nodes.List()) > 0 {
			return
		}
		refreshMu.Lock()
		defer refreshMu.Unlock()
		if _, err := forceRefreshNodes(ctx); err != nil {
			log.Printf("initial background node scan failed: %v", err)
		}
	}()
	startNodeUpdater(ctx, cfg, func(updateCtx context.Context) error {
		refreshMu.Lock()
		defer refreshMu.Unlock()
		_, err := forceRefreshNodes(updateCtx)
		return err
	})
	if err := database.ResetRunningDeepTestJobs(ctx); err != nil {
		log.Printf("reset running deep test jobs failed: %v", err)
	}
	deepWorker := deeptest.NewWorker(deeptest.Config{
		Queue:       database,
		Nodes:       nodes,
		Tester:      deeptest.OpenVPNTester{DataDir: cfg.DataDir, Command: cfg.OpenVPNCommand},
		BatchSize:   1,
		Concurrency: 1,
		Interval:    3 * time.Second,
		Timeout:     25 * time.Second,
	})
	go deepWorker.Run(ctx)

	proxyRuntime := newProxyRuntime(cfg.ProxyUsername, cfg.ProxyPassword, manager, tracker)
	if err := proxyRuntime.Sync(ctx, channels); err != nil {
		_ = database.Close()
		return services{}, err
	}
	gatewayRuntime := newProxyRuntime(cfg.ProxyUsername, cfg.ProxyPassword, manager, tracker)
	if err := gatewayRuntime.Sync(ctx, gatewayChannels(cfg)); err != nil {
		_ = database.Close()
		return services{}, err
	}
	reloadRuntime := func(reloadCtx context.Context) error {
		nextCfg, err := config.Load(cfgPath)
		if err == nil {
			proxyRuntime.SetCredentials(nextCfg.ProxyUsername, nextCfg.ProxyPassword)
		} else {
			log.Printf("reload config for proxy credentials failed: %v", err)
		}
		nextChannels, err := database.ListChannels(reloadCtx)
		if err != nil {
			return err
		}
		nextManagerChannels := append(append([]config.Channel(nil), nextChannels...), gatewayChannels(nextCfg)...)
		if err := manager.ReplaceChannels(ctx, nextManagerChannels); err != nil {
			return err
		}
		if err := proxyRuntime.Sync(ctx, nextChannels); err != nil {
			return err
		}
		return gatewayRuntime.Sync(ctx, gatewayChannels(nextCfg))
	}

	return services{
		admin: admin.NewServer(manager, nodes, tracker,
			admin.WithConfig(cfgPath, cfg),
			admin.WithStorage(database),
			admin.WithNodeRefresher(func(ctx context.Context) ([]node.Node, error) {
				refreshMu.Lock()
				defer refreshMu.Unlock()
				loaded, err := forceRefreshNodes(ctx)
				if err != nil {
					return nil, err
				}
				return loaded, nil
			}),
			admin.WithNodeChecker(nodecheck.Checker{Timeout: 3 * time.Second}.Check),
			admin.WithRestarter(func(ctx context.Context) error {
				go func() {
					time.Sleep(200 * time.Millisecond)
					process, err := os.FindProcess(os.Getpid())
					if err != nil {
						log.Printf("find process for restart: %v", err)
						os.Exit(0)
						return
					}
					if err := process.Signal(syscall.SIGTERM); err != nil {
						log.Printf("signal process for restart: %v", err)
						os.Exit(0)
					}
				}()
				return nil
			}),
			admin.WithRuntimeReloader(reloadRuntime),
			admin.WithNodeScanStatus(scanState.snapshot),
		),
		nodes:          nodes,
		channels:       manager,
		tracker:        tracker,
		storage:        database,
		proxyRuntime:   proxyRuntime,
		gatewayRuntime: gatewayRuntime,
		configPath:     cfgPath,
	}, nil
}

func gatewayChannels(cfg config.Config) []config.Channel {
	return []config.Channel{
		{ID: "rotating-gateway", ListenHost: cfg.RotatingGatewayHost, ListenPort: cfg.RotatingGatewayPort, Region: "*", SelectionMode: config.SelectionAuto, RotateOnDial: true, Enabled: true},
		{ID: "extract-api-proxy", ListenHost: cfg.ProxyExtractAPIHost, ListenPort: cfg.ProxyExtractAPIPort, Region: "*", SelectionMode: config.SelectionAuto, RotateOnDial: true, Enabled: true},
	}
}

func loadNodes(ctx context.Context, cfg config.Config) ([]node.Node, error) {
	return loadNodesWithProgress(ctx, cfg, nil)
}

func loadNodesWithProgress(ctx context.Context, cfg config.Config, progress func(total int, success int, failed int)) ([]node.Node, error) {
	if cfg.TunnelBackend == config.TunnelBackendFake {
		nodes := demoNodes()
		if progress != nil {
			progress(len(nodes), len(nodes), 0)
		}
		return nodes, nil
	}
	fetched, err := (vpngate.Client{}).Fetch(ctx)
	if err != nil {
		return nil, err
	}
	if len(fetched) == 0 {
		return nil, fmt.Errorf("vpngate returned no nodes")
	}
	prefiltered := prefilterNodes(ctx, fetched, nodecheck.Checker{Timeout: 3 * time.Second}, 8)
	if len(prefiltered) == 0 {
		return nil, fmt.Errorf("no nodes passed lightweight prefilter")
	}
	enriched, err := (ipinfo.Client{}).Enrich(ctx, prefiltered)
	if err != nil {
		log.Printf("ip info enrich failed: %v", err)
		return filterConnectableNodes(ctx, prefiltered, openVPNNodeConnectivityTester{tester: deeptest.OpenVPNTester{DataDir: cfg.DataDir, Command: cfg.OpenVPNCommand}}, progress)
	}
	return filterConnectableNodes(ctx, enriched, openVPNNodeConnectivityTester{tester: deeptest.OpenVPNTester{DataDir: cfg.DataDir, Command: cfg.OpenVPNCommand}}, progress)
}

func prefilterNodes(ctx context.Context, nodes []node.Node, checker nodecheck.Checker, workers int) []node.Node {
	if workers <= 0 {
		workers = 4
	}
	type result struct {
		index int
		node  node.Node
		keep  bool
	}
	jobs := make(chan int)
	results := make(chan result, len(nodes))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				checked := checker.Check(ctx, nodes[index])
				results <- result{index: index, node: checked, keep: checked.Available}
			}
		}()
	}
	for index := range nodes {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			close(results)
			return nil
		case jobs <- index:
		}
	}
	close(jobs)
	wg.Wait()
	close(results)

	byIndex := make(map[int]node.Node, len(nodes))
	for result := range results {
		if result.keep {
			byIndex[result.index] = result.node
		}
	}
	filtered := make([]node.Node, 0, len(byIndex))
	for index := range nodes {
		if n, ok := byIndex[index]; ok {
			filtered = append(filtered, n)
		}
	}
	return filtered
}

type nodeConnectivityTester interface {
	Test(ctx context.Context, n node.Node) error
}

type nodeConnectivityTesterFunc func(ctx context.Context, n node.Node) error

func (f nodeConnectivityTesterFunc) Test(ctx context.Context, n node.Node) error {
	return f(ctx, n)
}

type openVPNNodeConnectivityTester struct {
	tester deeptest.OpenVPNTester
}

func (t openVPNNodeConnectivityTester) Test(ctx context.Context, n node.Node) error {
	result := t.tester.Test(ctx, n)
	if result.Status != deeptest.StatusSuccess {
		if result.FailReason != "" {
			return fmt.Errorf(result.FailReason)
		}
		return fmt.Errorf("openvpn connectivity test failed")
	}
	return nil
}

func filterConnectableNodes(ctx context.Context, nodes []node.Node, tester nodeConnectivityTester, progress ...func(total int, success int, failed int)) ([]node.Node, error) {
	if tester == nil {
		return append([]node.Node(nil), nodes...), nil
	}
	var progressFn func(total int, success int, failed int)
	if len(progress) > 0 {
		progressFn = progress[0]
	}
	type result struct {
		index int
		node  node.Node
		ok    bool
	}
	const workers = 4
	jobs := make(chan int)
	results := make(chan result, len(nodes))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				n := nodes[index]
				testCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
				err := tester.Test(testCtx, n)
				cancel()
				if err != nil {
					log.Printf("drop unreachable node %s: %v", n.ID, err)
					results <- result{index: index}
					continue
				}
				n.Available = true
				n.ProbeStatus = "available"
				n.ProbeMessage = "openvpn connectivity verified"
				n.FailReason = ""
				n.LastTestedAt = time.Now()
				n.ProbedAt = n.LastTestedAt
				results <- result{index: index, node: n, ok: true}
			}
		}()
	}
	for index := range nodes {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			close(results)
			return nil, ctx.Err()
		case jobs <- index:
		}
	}
	close(jobs)
	wg.Wait()
	close(results)

	byIndex := make(map[int]node.Node, len(nodes))
	success := 0
	failed := 0
	for result := range results {
		if result.ok {
			byIndex[result.index] = result.node
			success++
		} else {
			failed++
		}
		if progressFn != nil {
			progressFn(len(nodes), success, failed)
		}
	}
	filtered := make([]node.Node, 0, len(byIndex))
	for index := range nodes {
		if n, ok := byIndex[index]; ok {
			filtered = append(filtered, n)
		}
	}
	return filtered, nil
}

func freshNodes(nodes []node.Node, ttl time.Duration, now time.Time) []node.Node {
	if ttl <= 0 {
		return append([]node.Node(nil), nodes...)
	}
	fresh := make([]node.Node, 0, len(nodes))
	for _, n := range nodes {
		if n.LastTestedAt.IsZero() {
			continue
		}
		if now.Sub(n.LastTestedAt) <= ttl {
			fresh = append(fresh, n)
		}
	}
	return fresh
}

func latestNodeTestedAt(nodes []node.Node) time.Time {
	var latest time.Time
	for _, n := range nodes {
		if n.LastTestedAt.After(latest) {
			latest = n.LastTestedAt
		}
	}
	return latest
}

type proxyRuntime struct {
	mu            sync.Mutex
	proxyUsername string
	proxyPassword string
	dialer        proxy.Dialer
	tracker       *connection.Tracker
	entries       map[string]*proxyRuntimeEntry
}

type proxyRuntimeEntry struct {
	channel  config.Channel
	server   *proxy.Server
	listener net.Listener
}

func newProxyRuntime(proxyUsername string, proxyPassword string, dialer proxy.Dialer, tracker *connection.Tracker) *proxyRuntime {
	return &proxyRuntime{
		proxyUsername: proxyUsername,
		proxyPassword: proxyPassword,
		dialer:        dialer,
		tracker:       tracker,
		entries:       map[string]*proxyRuntimeEntry{},
	}
}

func (r *proxyRuntime) Sync(ctx context.Context, channels []config.Channel) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	next := make(map[string]config.Channel, len(channels))
	for _, ch := range channels {
		if ch.Enabled {
			next[ch.ID] = ch
		}
	}
	for id, entry := range r.entries {
		ch, ok := next[id]
		if !ok || proxyListenAddr(ch) != proxyListenAddr(entry.channel) {
			_ = entry.listener.Close()
			delete(r.entries, id)
		}
	}
	for id, ch := range next {
		if _, ok := r.entries[id]; ok {
			r.entries[id].channel = ch
			continue
		}
		addr := proxyListenAddr(ch)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("proxy listen on %s: %w", addr, err)
		}
		server := proxy.NewServer(addr, ch.ID, r.proxyUsername, r.proxyPassword, r.dialer, r.tracker)
		entry := &proxyRuntimeEntry{channel: ch, server: server, listener: listener}
		r.entries[id] = entry
		go func(entry *proxyRuntimeEntry) {
			if err := entry.server.Serve(entry.listener); err != nil {
				log.Printf("proxy channel %s stopped: %v", entry.server.ChannelID, err)
			}
		}(entry)
		log.Printf("proxy channel %s listening on %s", ch.ID, addr)
	}
	return nil
}

func (r *proxyRuntime) SetCredentials(username string, password string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.proxyUsername = username
	r.proxyPassword = password
	for _, entry := range r.entries {
		entry.server.SetCredentials(username, password)
	}
}

func (r *proxyRuntime) Stop(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, entry := range r.entries {
		_ = entry.listener.Close()
		delete(r.entries, id)
	}
	return nil
}

func proxyListenAddr(ch config.Channel) string {
	return fmt.Sprintf("%s:%d", ch.ListenHost, ch.ListenPort)
}

func startNodeUpdater(ctx context.Context, cfg config.Config, refresh func(context.Context) error) {
	if refresh == nil {
		return
	}
	interval, err := config.ParseNodeRefreshInterval(cfg.NodeRefreshInterval)
	if err != nil {
		log.Printf("node refresh interval disabled: %v", err)
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := refresh(ctx); err != nil {
					log.Printf("scheduled node update failed: %v", err)
					continue
				}
				log.Printf("scheduled node update completed")
			}
		}
	}()
}

type channelHistoryAdapter struct {
	store *storage.Store
}

func (a channelHistoryAdapter) RecentNodeIDsForChannel(ctx context.Context, channelID string, since time.Time) (map[string]time.Time, error) {
	return a.store.RecentNodeIDsForChannel(ctx, channelID, since)
}

func (a channelHistoryAdapter) DeepTestResults(ctx context.Context) (map[string]deeptest.Result, error) {
	return a.store.DeepTestResults(ctx)
}

func (a channelHistoryAdapter) RecordChannelNodeUse(ctx context.Context, use channel.NodeUse) error {
	return a.store.RecordChannelNodeUse(ctx, storage.ChannelNodeUse{
		ChannelID:   use.ChannelID,
		NodeID:      use.NodeID,
		ExitIP:      use.ExitIP,
		ConnectedAt: use.ConnectedAt,
		SwitchedAt:  use.SwitchedAt,
	})
}

func tunnelFactory(cfg config.Config) channel.TunnelFactory {
	switch cfg.TunnelBackend {
	case config.TunnelBackendOpenVPN:
		return func(name string) tunnel.Tunnel {
			return tunnel.NewOpenVPN(tunnel.OpenVPNConfig{DataDir: cfg.DataDir, Command: cfg.OpenVPNCommand})
		}
	default:
		return func(name string) tunnel.Tunnel {
			return tunnel.NewFake(name)
		}
	}
}

func demoNodes() []node.Node {
	now := time.Now()
	return []node.Node{
		{ID: "jp-demo", Region: "jp", IP: "203.0.113.10", Hostname: "jp-demo", Port: 1194, Proto: "udp", LatencyMS: 50, Speed: 1000, Available: true, LastTestedAt: now, ProbedAt: now, ProbeStatus: "available", ProbeMessage: "demo node", IPType: "residential", Quality: "normal", PurityScore: 90, Owner: "Demo Home ISP"},
		{ID: "us-demo", Region: "us", IP: "198.51.100.10", Hostname: "us-demo", Port: 443, Proto: "tcp", LatencyMS: 60, Speed: 900, Available: true, LastTestedAt: now, ProbedAt: now, ProbeStatus: "available", ProbeMessage: "demo node", IPType: "hosting", Quality: "datacenter", PurityScore: 45, Owner: "Demo Cloud"},
	}
}
