package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/config"
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
		ch.Enabled = enabled != 0
		channels = append(channels, ch)
	}
	return channels, rows.Err()
}

func (s *Store) SaveChannel(ctx context.Context, originalID string, ch config.Channel) error {
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
