package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/config"
	"github.com/iceqi/region-proxy-gateway/internal/deeptest"
	"github.com/iceqi/region-proxy-gateway/internal/node"
	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create database dir: %w", err)
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	statements := []string{
		`PRAGMA journal_mode = WAL`,
		`CREATE TABLE IF NOT EXISTS meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS channels (
			id TEXT PRIMARY KEY,
			listen_host TEXT NOT NULL,
			listen_port INTEGER NOT NULL,
			region TEXT NOT NULL,
			rotate_minutes INTEGER NOT NULL,
			selection_mode TEXT NOT NULL,
			manual_node_id TEXT NOT NULL,
			enabled INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY,
			region TEXT NOT NULL,
			country TEXT NOT NULL,
			ip TEXT NOT NULL,
			hostname TEXT NOT NULL,
			port INTEGER NOT NULL,
			proto TEXT NOT NULL,
			openvpn TEXT NOT NULL,
			latency_ms INTEGER NOT NULL,
			speed INTEGER NOT NULL,
			available INTEGER NOT NULL,
			last_tested_at TEXT NOT NULL,
			fail_reason TEXT NOT NULL,
			owner TEXT NOT NULL,
			asn TEXT NOT NULL,
			as_name TEXT NOT NULL,
			location TEXT NOT NULL,
			ip_type TEXT NOT NULL,
			quality TEXT NOT NULL,
			purity_score INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS deep_test_jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			node_id TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			fail_reason TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_deep_test_jobs_status_created ON deep_test_jobs(status, created_at)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_deep_test_jobs_pending_node ON deep_test_jobs(node_id) WHERE status IN ('pending', 'running')`,
		`CREATE TABLE IF NOT EXISTS deep_test_results (
			node_id TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			exit_ip TEXT NOT NULL,
			exit_country TEXT NOT NULL,
			connect_ms INTEGER NOT NULL,
			tested_at TEXT NOT NULL,
			fail_reason TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS channel_node_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			channel_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			exit_ip TEXT NOT NULL,
			connected_at TEXT NOT NULL,
			switched_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_channel_node_history_channel_switched ON channel_node_history(channel_id, switched_at)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) MigrateChannels(ctx context.Context, channels []config.Channel) error {
	if len(channels) == 0 {
		return nil
	}
	var done string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'channels_migrated'`).Scan(&done); err == nil && done == "1" {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, ch := range channels {
		if err := saveChannelTx(ctx, tx, "", ch); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO meta(key, value) VALUES('channels_migrated', '1')`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListChannels(ctx context.Context) ([]config.Channel, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, listen_host, listen_port, region, rotate_minutes, selection_mode, manual_node_id, enabled FROM channels ORDER BY listen_port, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []config.Channel
	for rows.Next() {
		var ch config.Channel
		var enabled int
		if err := rows.Scan(&ch.ID, &ch.ListenHost, &ch.ListenPort, &ch.Region, &ch.RotateMinutes, &ch.SelectionMode, &ch.ManualNodeID, &enabled); err != nil {
			return nil, err
		}
		ch.Region = strings.ToLower(strings.TrimSpace(ch.Region))
		ch.Enabled = enabled != 0
		channels = append(channels, ch)
	}
	return channels, rows.Err()
}

func (s *Store) SaveChannel(ctx context.Context, originalID string, ch config.Channel) error {
	ch.Region = strings.ToLower(strings.TrimSpace(ch.Region))
	if err := ch.Validate(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := saveChannelTx(ctx, tx, originalID, ch); err != nil {
		return err
	}
	return tx.Commit()
}

func saveChannelTx(ctx context.Context, tx *sql.Tx, originalID string, ch config.Channel) error {
	ch.Region = strings.ToLower(strings.TrimSpace(ch.Region))
	enabled := 0
	if ch.Enabled {
		enabled = 1
	}
	if originalID != "" && originalID != ch.ID {
		if _, err := tx.ExecContext(ctx, `DELETE FROM channels WHERE id = ?`, originalID); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO channels(id, listen_host, listen_port, region, rotate_minutes, selection_mode, manual_node_id, enabled)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			listen_host = excluded.listen_host,
			listen_port = excluded.listen_port,
			region = excluded.region,
			rotate_minutes = excluded.rotate_minutes,
			selection_mode = excluded.selection_mode,
			manual_node_id = excluded.manual_node_id,
			enabled = excluded.enabled
	`, ch.ID, ch.ListenHost, ch.ListenPort, ch.Region, ch.RotateMinutes, ch.SelectionMode, ch.ManualNodeID, enabled)
	return err
}

func (s *Store) DeleteChannel(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM channels WHERE id = ?`, id)
	return err
}

func (s *Store) ReplaceNodes(ctx context.Context, nodes []node.Node) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM nodes`); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO nodes(id, region, country, ip, hostname, port, proto, openvpn, latency_ms, speed, available, last_tested_at, fail_reason, owner, asn, as_name, location, ip_type, quality, purity_score)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, n := range nodes {
		available := 0
		if n.Available {
			available = 1
		}
		if _, err := stmt.ExecContext(ctx, n.ID, n.Region, n.Country, n.IP, n.Hostname, n.Port, n.Proto, n.OpenVPN, 0, n.Speed, available, encodeTime(n.LastTestedAt), n.FailReason, n.Owner, n.ASN, n.ASName, n.Location, n.IPType, n.Quality, n.PurityScore); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListNodes(ctx context.Context) ([]node.Node, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, region, country, ip, hostname, port, proto, openvpn, latency_ms, speed, available, last_tested_at, fail_reason, owner, asn, as_name, location, ip_type, quality, purity_score FROM nodes ORDER BY region, latency_ms, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []node.Node
	for rows.Next() {
		var n node.Node
		var available int
		var lastTestedAt string
		if err := rows.Scan(&n.ID, &n.Region, &n.Country, &n.IP, &n.Hostname, &n.Port, &n.Proto, &n.OpenVPN, &n.LatencyMS, &n.Speed, &available, &lastTestedAt, &n.FailReason, &n.Owner, &n.ASN, &n.ASName, &n.Location, &n.IPType, &n.Quality, &n.PurityScore); err != nil {
			return nil, err
		}
		n.Available = available != 0
		n.LatencyMS = 0
		n.LastTestedAt = decodeTime(lastTestedAt)
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

type ChannelNodeUse struct {
	ChannelID   string
	NodeID      string
	ExitIP      string
	ConnectedAt time.Time
	SwitchedAt  time.Time
}

func (s *Store) EnqueueDeepTestJobs(ctx context.Context, nodeIDs []string) (deeptest.EnqueueSummary, error) {
	now := encodeTime(time.Now())
	summary := deeptest.EnqueueSummary{}
	seen := map[string]struct{}{}
	for _, nodeID := range nodeIDs {
		nodeID = strings.TrimSpace(nodeID)
		if nodeID == "" {
			summary.Skipped++
			continue
		}
		if _, ok := seen[nodeID]; ok {
			summary.Skipped++
			continue
		}
		seen[nodeID] = struct{}{}
		res, err := s.db.ExecContext(ctx, `
			INSERT OR IGNORE INTO deep_test_jobs(node_id, status, created_at, updated_at)
			VALUES(?, ?, ?, ?)
		`, nodeID, deeptest.StatusPending, now, now)
		if err != nil {
			return summary, err
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			summary.Skipped++
		} else {
			summary.Created++
		}
	}
	return summary, nil
}

func (s *Store) ClaimDeepTestJobs(ctx context.Context, limit int, now time.Time) ([]deeptest.Job, error) {
	if limit <= 0 {
		limit = 1
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
		SELECT id, node_id, status, created_at, updated_at
		FROM deep_test_jobs
		WHERE status = ?
		ORDER BY created_at, id
		LIMIT ?
	`, deeptest.StatusPending, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := []deeptest.Job{}
	for rows.Next() {
		var job deeptest.Job
		var createdAt, updatedAt string
		if err := rows.Scan(&job.ID, &job.NodeID, &job.Status, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		job.CreatedAt = decodeTime(createdAt)
		job.UpdatedAt = decodeTime(updatedAt)
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range jobs {
		if _, err := tx.ExecContext(ctx, `UPDATE deep_test_jobs SET status = ?, updated_at = ? WHERE id = ?`, deeptest.StatusRunning, encodeTime(now), jobs[i].ID); err != nil {
			return nil, err
		}
		jobs[i].Status = deeptest.StatusRunning
		jobs[i].UpdatedAt = now
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (s *Store) CompleteDeepTestJob(ctx context.Context, jobID int64, result deeptest.Result) error {
	if result.TestedAt.IsZero() {
		result.TestedAt = time.Now()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO deep_test_results(node_id, status, exit_ip, exit_country, connect_ms, tested_at, fail_reason)
		VALUES(?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(node_id) DO UPDATE SET
			status = excluded.status,
			exit_ip = excluded.exit_ip,
			exit_country = excluded.exit_country,
			connect_ms = excluded.connect_ms,
			tested_at = excluded.tested_at,
			fail_reason = excluded.fail_reason
	`, result.NodeID, result.Status, result.ExitIP, result.ExitCountry, result.ConnectMS, encodeTime(result.TestedAt), result.FailReason); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE deep_test_jobs SET status = ?, updated_at = ?, fail_reason = ? WHERE id = ?`, result.Status, encodeTime(result.TestedAt), result.FailReason, jobID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeepTestResult(ctx context.Context, nodeID string) (deeptest.Result, bool, error) {
	var result deeptest.Result
	var testedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT node_id, status, exit_ip, exit_country, connect_ms, tested_at, fail_reason
		FROM deep_test_results
		WHERE node_id = ?
	`, nodeID).Scan(&result.NodeID, &result.Status, &result.ExitIP, &result.ExitCountry, &result.ConnectMS, &testedAt, &result.FailReason)
	if err == sql.ErrNoRows {
		return deeptest.Result{}, false, nil
	}
	if err != nil {
		return deeptest.Result{}, false, err
	}
	result.TestedAt = decodeTime(testedAt)
	return result, true, nil
}

func (s *Store) DeepTestResults(ctx context.Context) (map[string]deeptest.Result, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT node_id, status, exit_ip, exit_country, connect_ms, tested_at, fail_reason
		FROM deep_test_results
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := map[string]deeptest.Result{}
	for rows.Next() {
		var result deeptest.Result
		var testedAt string
		if err := rows.Scan(&result.NodeID, &result.Status, &result.ExitIP, &result.ExitCountry, &result.ConnectMS, &testedAt, &result.FailReason); err != nil {
			return nil, err
		}
		result.TestedAt = decodeTime(testedAt)
		results[result.NodeID] = result
	}
	return results, rows.Err()
}

func (s *Store) DeepTestQueueStats(ctx context.Context) (deeptest.QueueStats, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM deep_test_jobs GROUP BY status`)
	if err != nil {
		return deeptest.QueueStats{}, err
	}
	defer rows.Close()

	var stats deeptest.QueueStats
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return deeptest.QueueStats{}, err
		}
		switch status {
		case deeptest.StatusPending:
			stats.Pending = count
		case deeptest.StatusRunning:
			stats.Running = count
		case deeptest.StatusSuccess:
			stats.Success = count
		case deeptest.StatusFailed:
			stats.Failed = count
		}
	}
	return stats, rows.Err()
}

func (s *Store) ResetRunningDeepTestJobs(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE deep_test_jobs SET status = ?, updated_at = ? WHERE status = ?`, deeptest.StatusPending, encodeTime(time.Now()), deeptest.StatusRunning)
	return err
}

func (s *Store) RecordChannelNodeUse(ctx context.Context, use ChannelNodeUse) error {
	if use.ConnectedAt.IsZero() {
		use.ConnectedAt = time.Now()
	}
	if use.SwitchedAt.IsZero() {
		use.SwitchedAt = use.ConnectedAt
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO channel_node_history(channel_id, node_id, exit_ip, connected_at, switched_at)
		VALUES(?, ?, ?, ?, ?)
	`, use.ChannelID, use.NodeID, use.ExitIP, encodeTime(use.ConnectedAt), encodeTime(use.SwitchedAt))
	return err
}

func (s *Store) RecentNodeIDsForChannel(ctx context.Context, channelID string, since time.Time) (map[string]time.Time, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT node_id, MAX(switched_at)
		FROM channel_node_history
		WHERE channel_id = ? AND switched_at >= ?
		GROUP BY node_id
	`, channelID, encodeTime(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	recent := map[string]time.Time{}
	for rows.Next() {
		var nodeID, switchedAt string
		if err := rows.Scan(&nodeID, &switchedAt); err != nil {
			return nil, err
		}
		recent[nodeID] = decodeTime(switchedAt)
	}
	return recent, rows.Err()
}

func (s *Store) CurrentChannelNodeUse(ctx context.Context, channelID string) (ChannelNodeUse, bool, error) {
	var use ChannelNodeUse
	var connectedAt, switchedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT channel_id, node_id, exit_ip, connected_at, switched_at
		FROM channel_node_history
		WHERE channel_id = ?
		ORDER BY switched_at DESC, id DESC
		LIMIT 1
	`, channelID).Scan(&use.ChannelID, &use.NodeID, &use.ExitIP, &connectedAt, &switchedAt)
	if err == sql.ErrNoRows {
		return ChannelNodeUse{}, false, nil
	}
	if err != nil {
		return ChannelNodeUse{}, false, err
	}
	use.ConnectedAt = decodeTime(connectedAt)
	use.SwitchedAt = decodeTime(switchedAt)
	return use, true, nil
}

func encodeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func decodeTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339Nano, value)
	return t
}
