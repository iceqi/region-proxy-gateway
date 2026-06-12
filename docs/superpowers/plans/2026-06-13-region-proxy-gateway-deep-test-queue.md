# Region Proxy Gateway Deep Test Queue Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add queued OpenVPN deep testing, persist test results and channel node history, keep admin APIs lightweight, and make rotation avoid recently used nodes for 24 hours.

**Architecture:** SQLite owns durable queue, deep-test result cache, and channel usage history. A low-concurrency Go worker runs in background goroutines, starts one temporary OpenVPN process per job with `context` timeouts, records results, and always cleans up. Admin list APIs return compact views without raw OpenVPN config text, while channel selection reads deep-test/cache/history metadata to prefer verified nodes and avoid the current and recently used nodes.

**Tech Stack:** Go standard library (`context`, `database/sql`, `os/exec`, `sync`, `time`, `net/http`), existing packages (`internal/storage`, `internal/node`, `internal/channel`, `internal/admin`, `internal/tunnel`), SQLite via `github.com/mattn/go-sqlite3`.

---

## File Structure

- Modify `internal/storage/sqlite.go`: add queue/result/history tables and methods.
- Modify `internal/storage/sqlite_test.go`: cover migrations, enqueue dedupe, result persistence, history queries.
- Create `internal/deeptest/types.go`: shared deep-test job/result types.
- Create `internal/deeptest/worker.go`: background worker, queue polling, timeout handling.
- Create `internal/deeptest/worker_test.go`: worker success/failure tests with fake tester.
- Create `internal/deeptest/openvpn.go`: OpenVPN-backed tester using temporary tunnel process.
- Create `internal/deeptest/openvpn_test.go`: command/config cleanup tests using fake process starter.
- Modify `internal/admin/server.go`: add compact node views, queue APIs, and deep-test result fields in nodes.
- Modify `internal/admin/server_test.go`: admin endpoint and HTML coverage.
- Modify `internal/admin/static.go`: add deep-test button, queue status polling, deep-test status column.
- Modify `internal/channel/manager.go`: inject node metadata/history provider and record history on start/switch/rotation.
- Modify `internal/channel/manager_test.go`: 24-hour avoid and fallback behavior.
- Modify `cmd/region-proxy-gateway/main.go`: start deep-test worker and wire storage/history into manager/admin.
- Modify `README.md`: document deep testing and rotation avoid behavior.

---

## Task 1: SQLite Queue, Result, And History Storage

**Files:**
- Modify: `internal/storage/sqlite.go`
- Modify: `internal/storage/sqlite_test.go`
- Create: `internal/deeptest/types.go`

- [ ] **Step 1: Add failing storage tests**

Add these tests to `internal/storage/sqlite_test.go`:

```go
func TestSQLiteStoreDeepTestQueueDeduplicatesPendingJobs(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	first, err := store.EnqueueDeepTestJobs(ctx, []string{"jp-1", "jp-1", "jp-2"})
	if err != nil {
		t.Fatalf("EnqueueDeepTestJobs first: %v", err)
	}
	second, err := store.EnqueueDeepTestJobs(ctx, []string{"jp-1", "jp-3"})
	if err != nil {
		t.Fatalf("EnqueueDeepTestJobs second: %v", err)
	}

	if first.Created != 2 || first.Skipped != 1 {
		t.Fatalf("first summary = %+v, want 2 created 1 skipped", first)
	}
	if second.Created != 1 || second.Skipped != 1 {
		t.Fatalf("second summary = %+v, want 1 created 1 skipped", second)
	}

	jobs, err := store.ClaimDeepTestJobs(ctx, 10, time.Now())
	if err != nil {
		t.Fatalf("ClaimDeepTestJobs: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("jobs = %d, want 3", len(jobs))
	}
}

func TestSQLiteStoreSavesDeepTestResult(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	if _, err := store.EnqueueDeepTestJobs(ctx, []string{"jp-1"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	jobs, err := store.ClaimDeepTestJobs(ctx, 1, time.Now())
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(jobs))
	}

	result := deeptest.Result{
		NodeID:      "jp-1",
		Status:      deeptest.StatusSuccess,
		ExitIP:      "203.0.113.99",
		ExitCountry: "Japan",
		ConnectMS:   1234,
		TestedAt:    time.Date(2026, 6, 13, 1, 2, 3, 0, time.UTC),
	}
	if err := store.CompleteDeepTestJob(ctx, jobs[0].ID, result); err != nil {
		t.Fatalf("CompleteDeepTestJob: %v", err)
	}

	got, ok, err := store.DeepTestResult(ctx, "jp-1")
	if err != nil {
		t.Fatalf("DeepTestResult: %v", err)
	}
	if !ok || got.Status != deeptest.StatusSuccess || got.ExitIP != "203.0.113.99" || got.ConnectMS != 1234 {
		t.Fatalf("result = %+v ok=%v, want saved success", got, ok)
	}
}

func TestSQLiteStoreRecordsChannelHistoryAndRecentUse(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)

	if err := store.RecordChannelNodeUse(ctx, storage.ChannelNodeUse{
		ChannelID:   "jp-3000",
		NodeID:      "jp-1",
		ExitIP:      "203.0.113.10",
		ConnectedAt: now.Add(-2 * time.Hour),
		SwitchedAt:  now.Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("RecordChannelNodeUse recent: %v", err)
	}
	if err := store.RecordChannelNodeUse(ctx, storage.ChannelNodeUse{
		ChannelID:   "jp-3000",
		NodeID:      "jp-old",
		ExitIP:      "203.0.113.11",
		ConnectedAt: now.Add(-30 * time.Hour),
		SwitchedAt:  now.Add(-30 * time.Hour),
	}); err != nil {
		t.Fatalf("RecordChannelNodeUse old: %v", err)
	}

	recent, err := store.RecentNodeIDsForChannel(ctx, "jp-3000", now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("RecentNodeIDsForChannel: %v", err)
	}
	if len(recent) != 1 || recent["jp-1"].IsZero() {
		t.Fatalf("recent = %+v, want only jp-1", recent)
	}

	current, ok, err := store.CurrentChannelNodeUse(ctx, "jp-3000")
	if err != nil {
		t.Fatalf("CurrentChannelNodeUse: %v", err)
	}
	if !ok || current.NodeID != "jp-1" || current.ExitIP != "203.0.113.10" {
		t.Fatalf("current = %+v ok=%v, want jp-1", current, ok)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/storage
```

Expected: FAIL because `deeptest` package and storage methods do not exist.

- [ ] **Step 3: Add deep-test types**

Create `internal/deeptest/types.go`:

```go
package deeptest

import "time"

const (
	StatusPending = "pending"
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusFailed  = "failed"
)

type Job struct {
	ID        int64     `json:"id"`
	NodeID    string    `json:"node_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type EnqueueSummary struct {
	Created int `json:"created"`
	Skipped int `json:"skipped"`
}

type Result struct {
	NodeID      string    `json:"node_id"`
	Status      string    `json:"status"`
	ExitIP      string    `json:"exit_ip"`
	ExitCountry string    `json:"exit_country"`
	ConnectMS   int       `json:"connect_ms"`
	TestedAt    time.Time `json:"tested_at"`
	FailReason  string    `json:"fail_reason"`
}
```

- [ ] **Step 4: Implement SQLite tables and methods**

Modify `internal/storage/sqlite.go` imports:

```go
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
```

Add these tables in `migrate` after the existing `nodes` table statement:

```go
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
```

Add this type and methods near channel methods:

```go
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
	var jobs []deeptest.Job
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
		FROM deep_test_results WHERE node_id = ?
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
```

- [ ] **Step 5: Run storage tests**

Run:

```bash
go test ./internal/storage
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/deeptest/types.go internal/storage/sqlite.go internal/storage/sqlite_test.go
git commit -m "feat: add deep test storage"
```

---

## Task 2: Deep Test Worker With Go Concurrency

**Files:**
- Create: `internal/deeptest/worker.go`
- Create: `internal/deeptest/worker_test.go`

- [ ] **Step 1: Write worker tests**

Create `internal/deeptest/worker_test.go`:

```go
package deeptest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/node"
)

type fakeQueue struct {
	mu        sync.Mutex
	jobs      []Job
	completed []Result
}

func (q *fakeQueue) ClaimDeepTestJobs(ctx context.Context, limit int, now time.Time) ([]Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.jobs) == 0 {
		return nil, nil
	}
	if limit > len(q.jobs) {
		limit = len(q.jobs)
	}
	jobs := append([]Job(nil), q.jobs[:limit]...)
	q.jobs = q.jobs[limit:]
	return jobs, nil
}

func (q *fakeQueue) CompleteDeepTestJob(ctx context.Context, jobID int64, result Result) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.completed = append(q.completed, result)
	return nil
}

type fakeNodeSource map[string]node.Node

func (s fakeNodeSource) NodeByID(id string) (node.Node, bool) {
	n, ok := s[id]
	return n, ok
}

func TestWorkerProcessesSuccessAndFailure(t *testing.T) {
	queue := &fakeQueue{jobs: []Job{{ID: 1, NodeID: "ok"}, {ID: 2, NodeID: "bad"}}}
	source := fakeNodeSource{
		"ok":  {ID: "ok", OpenVPN: "client"},
		"bad": {ID: "bad", OpenVPN: "client"},
	}
	tester := TesterFunc(func(ctx context.Context, n node.Node) Result {
		if n.ID == "bad" {
			return Result{NodeID: n.ID, Status: StatusFailed, FailReason: "boom", TestedAt: time.Now()}
		}
		return Result{NodeID: n.ID, Status: StatusSuccess, ExitIP: "203.0.113.10", ConnectMS: 50, TestedAt: time.Now()}
	})

	worker := NewWorker(Config{Queue: queue, Nodes: source, Tester: tester, BatchSize: 2, Concurrency: 1, Timeout: time.Second})
	processed, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if processed != 2 {
		t.Fatalf("processed = %d, want 2", processed)
	}
	if len(queue.completed) != 2 {
		t.Fatalf("completed = %d, want 2", len(queue.completed))
	}
	if queue.completed[0].Status != StatusSuccess || queue.completed[1].Status != StatusFailed {
		t.Fatalf("completed = %+v", queue.completed)
	}
}

func TestWorkerMarksMissingNodeFailed(t *testing.T) {
	queue := &fakeQueue{jobs: []Job{{ID: 1, NodeID: "missing"}}}
	worker := NewWorker(Config{
		Queue: queue,
		Nodes: fakeNodeSource{},
		Tester: TesterFunc(func(ctx context.Context, n node.Node) Result {
			t.Fatalf("tester should not be called")
			return Result{}
		}),
		BatchSize: 1,
		Timeout:   time.Second,
	})

	processed, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if processed != 1 || len(queue.completed) != 1 || queue.completed[0].Status != StatusFailed {
		t.Fatalf("processed=%d completed=%+v, want failed missing node", processed, queue.completed)
	}
}

func TestWorkerConvertsTesterPanicToFailure(t *testing.T) {
	queue := &fakeQueue{jobs: []Job{{ID: 1, NodeID: "panic"}}}
	worker := NewWorker(Config{
		Queue: queue,
		Nodes: fakeNodeSource{"panic": {ID: "panic", OpenVPN: "client"}},
		Tester: TesterFunc(func(ctx context.Context, n node.Node) Result {
			panic(errors.New("panic failure"))
		}),
		BatchSize: 1,
		Timeout:   time.Second,
	})

	if _, err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(queue.completed) != 1 || queue.completed[0].Status != StatusFailed {
		t.Fatalf("completed = %+v, want failed", queue.completed)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/deeptest
```

Expected: FAIL because worker types do not exist.

- [ ] **Step 3: Implement worker**

Create `internal/deeptest/worker.go`:

```go
package deeptest

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/node"
)

type Queue interface {
	ClaimDeepTestJobs(context.Context, int, time.Time) ([]Job, error)
	CompleteDeepTestJob(context.Context, int64, Result) error
}

type NodeSource interface {
	NodeByID(id string) (node.Node, bool)
}

type Tester interface {
	Test(context.Context, node.Node) Result
}

type TesterFunc func(context.Context, node.Node) Result

func (f TesterFunc) Test(ctx context.Context, n node.Node) Result {
	return f(ctx, n)
}

type Config struct {
	Queue       Queue
	Nodes       NodeSource
	Tester      Tester
	BatchSize   int
	Concurrency int
	Timeout     time.Duration
	Interval    time.Duration
}

type Worker struct {
	cfg Config
}

func NewWorker(cfg Config) *Worker {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 1
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 20 * time.Second
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 3 * time.Second
	}
	return &Worker{cfg: cfg}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.cfg.Interval)
	defer ticker.Stop()
	for {
		if _, err := w.RunOnce(ctx); err != nil {
			// Worker errors are intentionally swallowed here; callers can log by wrapping Queue.
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	if w.cfg.Queue == nil || w.cfg.Nodes == nil || w.cfg.Tester == nil {
		return 0, fmt.Errorf("deep test worker requires queue, nodes, and tester")
	}
	jobs, err := w.cfg.Queue.ClaimDeepTestJobs(ctx, w.cfg.BatchSize, time.Now())
	if err != nil {
		return 0, err
	}
	if len(jobs) == 0 {
		return 0, nil
	}
	jobsCh := make(chan Job)
	var wg sync.WaitGroup
	workers := w.cfg.Concurrency
	if workers > len(jobs) {
		workers = len(jobs)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobsCh {
				w.processJob(ctx, job)
			}
		}()
	}
	for _, job := range jobs {
		jobsCh <- job
	}
	close(jobsCh)
	wg.Wait()
	return len(jobs), nil
}

func (w *Worker) processJob(ctx context.Context, job Job) {
	n, ok := w.cfg.Nodes.NodeByID(job.NodeID)
	if !ok {
		_ = w.cfg.Queue.CompleteDeepTestJob(ctx, job.ID, Result{NodeID: job.NodeID, Status: StatusFailed, TestedAt: time.Now(), FailReason: "node not found"})
		return
	}
	testCtx, cancel := context.WithTimeout(ctx, w.cfg.Timeout)
	defer cancel()
	result := safeTest(testCtx, w.cfg.Tester, n)
	if result.NodeID == "" {
		result.NodeID = job.NodeID
	}
	if result.Status == "" {
		result.Status = StatusFailed
		result.FailReason = "empty test result"
	}
	if result.TestedAt.IsZero() {
		result.TestedAt = time.Now()
	}
	_ = w.cfg.Queue.CompleteDeepTestJob(ctx, job.ID, result)
}

func safeTest(ctx context.Context, tester Tester, n node.Node) (result Result) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = Result{NodeID: n.ID, Status: StatusFailed, TestedAt: time.Now(), FailReason: fmt.Sprintf("panic: %v", recovered)}
		}
	}()
	return tester.Test(ctx, n)
}
```

- [ ] **Step 4: Run worker tests**

Run:

```bash
go test ./internal/deeptest
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/deeptest/worker.go internal/deeptest/worker_test.go
git commit -m "feat: add deep test worker"
```

---

## Task 3: OpenVPN Deep Tester

**Files:**
- Create: `internal/deeptest/openvpn.go`
- Create: `internal/deeptest/openvpn_test.go`
- Modify: `internal/tunnel/openvpn.go` only if command helpers need reuse.

- [ ] **Step 1: Write OpenVPN tester tests**

Create `internal/deeptest/openvpn_test.go`:

```go
package deeptest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/node"
	"github.com/iceqi/region-proxy-gateway/internal/tunnel"
)

type fakeProcess struct {
	pid        int
	terminated bool
}

func (p *fakeProcess) PID() int { return p.pid }
func (p *fakeProcess) Wait() error {
	<-time.After(50 * time.Millisecond)
	return nil
}
func (p *fakeProcess) Terminate() error {
	p.terminated = true
	return nil
}
func (p *fakeProcess) Kill() error {
	p.terminated = true
	return nil
}

type fakeStarter struct {
	command []string
	process *fakeProcess
}

func (s *fakeStarter) Start(ctx context.Context, command []string) (tunnel.OpenVPNProcess, error) {
	s.command = append([]string(nil), command...)
	s.process = &fakeProcess{pid: 123}
	return s.process, nil
}

func TestOpenVPNTesterFailsEmptyConfig(t *testing.T) {
	tester := OpenVPNTester{DataDir: t.TempDir(), Timeout: time.Second}
	result := tester.Test(context.Background(), node.Node{ID: "empty"})
	if result.Status != StatusFailed || !strings.Contains(result.FailReason, "empty openvpn config") {
		t.Fatalf("result = %+v, want empty config failure", result)
	}
}

func TestOpenVPNTesterStartsAndStopsProcess(t *testing.T) {
	starter := &fakeStarter{}
	tester := OpenVPNTester{
		DataDir: t.TempDir(),
		Command: "openvpn",
		Timeout: time.Second,
		Starter: starter,
		ExitIPChecker: func(ctx context.Context, deviceName string) (string, string, error) {
			return "203.0.113.99", "Japan", nil
		},
	}
	result := tester.Test(context.Background(), node.Node{ID: "jp-1", OpenVPN: "client\nremote 203.0.113.10 1194 udp\n"})
	if result.Status != StatusSuccess || result.ExitIP != "203.0.113.99" || result.ConnectMS <= 0 {
		t.Fatalf("result = %+v, want success with exit ip", result)
	}
	if starter.process == nil || !starter.process.terminated {
		t.Fatalf("expected process to be terminated after test")
	}
	if len(starter.command) == 0 || starter.command[0] != "openvpn" {
		t.Fatalf("command = %+v, want openvpn command", starter.command)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/deeptest -run OpenVPN
```

Expected: FAIL because `OpenVPNTester` does not exist.

- [ ] **Step 3: Implement OpenVPN tester**

Create `internal/deeptest/openvpn.go`:

```go
package deeptest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/node"
	"github.com/iceqi/region-proxy-gateway/internal/tunnel"
)

type ExitIPChecker func(ctx context.Context, deviceName string) (ip string, country string, err error)

type OpenVPNTester struct {
	DataDir       string
	Command       string
	Timeout       time.Duration
	Starter       tunnel.OpenVPNProcessStarter
	ExitIPChecker ExitIPChecker
}

func (t OpenVPNTester) Test(ctx context.Context, n node.Node) Result {
	started := time.Now()
	if n.OpenVPN == "" {
		return Result{NodeID: n.ID, Status: StatusFailed, TestedAt: time.Now(), FailReason: "empty openvpn config"}
	}
	if t.Timeout <= 0 {
		t.Timeout = 20 * time.Second
	}
	if t.Starter == nil {
		t.Starter = tunnel.ExecOpenVPNProcessStarter{}
	}
	if t.ExitIPChecker == nil {
		t.ExitIPChecker = defaultExitIPChecker
	}
	ctx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()

	dataDir := t.DataDir
	if dataDir == "" {
		dataDir = "."
	}
	sessionDir := filepath.Join(dataDir, "deep-tests", fmt.Sprintf("%s-%d", n.ID, time.Now().UnixNano()))
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		return Result{NodeID: n.ID, Status: StatusFailed, TestedAt: time.Now(), FailReason: "create session dir: " + err.Error()}
	}
	defer os.RemoveAll(sessionDir)

	configPath := filepath.Join(sessionDir, "client.ovpn")
	if err := os.WriteFile(configPath, []byte(n.OpenVPN), 0600); err != nil {
		return Result{NodeID: n.ID, Status: StatusFailed, TestedAt: time.Now(), FailReason: "write config: " + err.Error()}
	}
	deviceName := fmt.Sprintf("rpgtest%d", time.Now().UnixNano()%100000)
	process, err := t.Starter.Start(ctx, tunnel.OpenVPNCommand(t.Command, configPath, deviceName))
	if err != nil {
		return Result{NodeID: n.ID, Status: StatusFailed, TestedAt: time.Now(), FailReason: "start openvpn: " + err.Error()}
	}
	defer func() {
		_ = process.Terminate()
		done := make(chan struct{})
		go func() {
			_ = process.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = process.Kill()
		}
	}()

	ip, country, err := t.ExitIPChecker(ctx, deviceName)
	if err != nil {
		return Result{NodeID: n.ID, Status: StatusFailed, TestedAt: time.Now(), FailReason: "exit IP check failed: " + err.Error()}
	}
	elapsed := int(time.Since(started).Milliseconds())
	if elapsed < 1 {
		elapsed = 1
	}
	return Result{NodeID: n.ID, Status: StatusSuccess, ExitIP: ip, ExitCountry: country, ConnectMS: elapsed, TestedAt: time.Now()}
}

func defaultExitIPChecker(ctx context.Context, deviceName string) (string, string, error) {
	return "", "", fmt.Errorf("exit IP checker is not configured for device %s", deviceName)
}
```

- [ ] **Step 4: Run OpenVPN tester tests**

Run:

```bash
go test ./internal/deeptest
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/deeptest/openvpn.go internal/deeptest/openvpn_test.go
git commit -m "feat: add openvpn deep tester"
```

---

## Task 4: Node Store Lookup Adapter

**Files:**
- Modify: `internal/node/store.go`
- Modify: `internal/node/store_test.go`

- [ ] **Step 1: Add failing test**

Add to `internal/node/store_test.go`:

```go
func TestStoreNodeByIDReturnsCopy(t *testing.T) {
	store := NewStore()
	store.Replace([]Node{{ID: "jp-1", Region: "jp", IP: "203.0.113.10", Available: true}})

	got, ok := store.NodeByID("jp-1")
	if !ok {
		t.Fatal("expected jp-1")
	}
	got.IP = "mutated"

	again, _ := store.NodeByID("jp-1")
	if again.IP != "203.0.113.10" {
		t.Fatalf("stored node mutated to %q", again.IP)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run:

```bash
go test ./internal/node -run TestStoreNodeByIDReturnsCopy
```

Expected: FAIL because `NodeByID` does not exist.

- [ ] **Step 3: Implement lookup**

Add to `internal/node/store.go`:

```go
func (s *Store) NodeByID(id string) (Node, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, node := range s.nodes {
		if node.ID == id {
			return node, true
		}
	}
	return Node{}, false
}
```

- [ ] **Step 4: Run node tests**

Run:

```bash
go test ./internal/node
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/node/store.go internal/node/store_test.go
git commit -m "feat: add node lookup"
```

---

## Task 5: Admin Deep Test API And UI

**Files:**
- Modify: `internal/admin/server.go`
- Modify: `internal/admin/server_test.go`
- Modify: `internal/admin/static.go`

- [ ] **Step 1: Add failing admin tests**

Add to `internal/admin/server_test.go`:

```go
func TestAdminEnqueuesDeepTestJobs(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	store := openAdminTestStore(t)
	server := NewServer(manager, nodes, nil, WithStorage(store))

	req := httptest.NewRequest(http.MethodPost, "/admin/api/nodes/deep-test", bytes.NewBufferString(`{"node_ids":["jp-1","jp-1"]}`))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Created int `json:"created"`
		Skipped int `json:"skipped"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Created != 1 || body.Skipped != 1 {
		t.Fatalf("body = %+v, want 1 created 1 skipped", body)
	}
}

func TestAdminNodesIncludeDeepTestResults(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	store := openAdminTestStore(t)
	if err := store.CompleteDeepTestJob(context.Background(), 0, deeptest.Result{
		NodeID:    "jp-1",
		Status:    deeptest.StatusSuccess,
		ExitIP:    "203.0.113.99",
		ConnectMS: 1000,
		TestedAt:  time.Now(),
	}); err != nil {
		t.Fatalf("CompleteDeepTestJob: %v", err)
	}
	server := NewServer(manager, nodes, nil, WithStorage(store))

	req := httptest.NewRequest(http.MethodGet, "/admin/api/nodes", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"deep_test"`) || !strings.Contains(rec.Body.String(), `203.0.113.99`) {
		t.Fatalf("nodes response missing deep test result: %s", rec.Body.String())
	}
}
```

Update imports to include:

```go
"github.com/iceqi/region-proxy-gateway/internal/deeptest"
```

- [ ] **Step 2: Run admin tests to verify failure**

Run:

```bash
go test ./internal/admin -run 'TestAdmin(EnqueuesDeepTestJobs|NodesIncludeDeepTestResults)'
```

Expected: FAIL because API and node view do not exist.

- [ ] **Step 3: Implement API and response view**

Modify `internal/admin/server.go`:

Add imports:

```go
"github.com/iceqi/region-proxy-gateway/internal/deeptest"
```

Add route before single-node probe:

```go
if r.Method == http.MethodPost && r.URL.Path == "/api/nodes/deep-test" {
	s.handleEnqueueDeepTest(w, r)
	return
}
```

Add view type:

```go
type nodeView struct {
	node.Node
	DeepTest *deeptest.Result `json:"deep_test,omitempty"`
}
```

Replace `/api/nodes` handler body with:

```go
"nodes": s.nodeViewList(r.URL.Query().Get("region")),
```

Add methods:

```go
func (s *Server) handleEnqueueDeepTest(w http.ResponseWriter, r *http.Request) {
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
	summary, err := s.storage.EnqueueDeepTestJobs(r.Context(), cleanNodeIDs(body.NodeIDs, 500))
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, summary)
}

func (s *Server) nodeViewList(region string) []nodeView {
	nodes := s.nodeList(region)
	results := map[string]deeptest.Result{}
	if s.storage != nil {
		loaded, err := s.storage.DeepTestResults(context.Background())
		if err == nil {
			results = loaded
		}
	}
	views := make([]nodeView, 0, len(nodes))
	for _, n := range nodes {
		view := nodeView{Node: n}
		if result, ok := results[n.ID]; ok {
			resultCopy := result
			view.DeepTest = &resultCopy
		}
		views = append(views, view)
	}
	return views
}
```

- [ ] **Step 4: Update UI**

Modify `internal/admin/static.go`:

Add the button next to existing node buttons:

```html
<a-button :loading="deepTesting" @click="enqueueDeepTestVisibleNodes">深度测试当前列表</a-button>
```

Add `deepTesting: false` to data.

Add a node table column before status:

```js
{ title: '深测', key: 'deep', width: 160, customRender: ({ record }) => this.deepTestCell(record) },
```

Add methods:

```js
async enqueueDeepTestVisibleNodes() {
  const nodeIDs = this.visibleNodes.map(n => n.id).filter(Boolean).slice(0, 500);
  if (!nodeIDs.length) return message.warning('当前列表没有可深测的节点');
  this.deepTesting = true;
  try {
    const body = await this.request('nodes/deep-test', { method: 'POST', body: JSON.stringify({ node_ids: nodeIDs }) });
    message.success('已加入深测队列：新增 ' + (body.created || 0) + '，跳过 ' + (body.skipped || 0));
    await this.load(false);
  } catch (err) {
    message.error(err.message);
  } finally {
    this.deepTesting = false;
  }
},
deepTestCell(record) {
  const result = record.deep_test;
  if (!result) return h(antd.Tag, null, () => '未深测');
  if (result.status === 'success') {
    return h('div', [h(antd.Tag, { color: 'green' }, () => '深测成功'), h('div', { class: 'mono' }, result.exit_ip || '-'), h('div', { class: 'muted' }, result.connect_ms ? result.connect_ms + ' ms' : '')]);
  }
  if (result.status === 'running') return h(antd.Tag, { color: 'blue' }, () => '深测中');
  return h('div', [h(antd.Tag, { color: 'red' }, () => '深测失败'), h('div', { class: 'muted' }, result.fail_reason || '')]);
},
```

- [ ] **Step 5: Run admin tests**

Run:

```bash
go test ./internal/admin
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/admin/server.go internal/admin/server_test.go internal/admin/static.go
git commit -m "feat: enqueue deep tests from admin"
```

---

## Task 6: Wire Worker Into Main

**Files:**
- Modify: `cmd/region-proxy-gateway/main.go`

- [ ] **Step 1: Add worker startup**

Modify `cmd/region-proxy-gateway/main.go` imports:

```go
"github.com/iceqi/region-proxy-gateway/internal/deeptest"
```

After `startNodeUpdater(ctx, cfg, nodes, database)`, add:

```go
deepTester := deeptest.OpenVPNTester{
	DataDir: cfg.DataDir,
	Command: cfg.OpenVPNCommand,
	Timeout: 20 * time.Second,
}
deepWorker := deeptest.NewWorker(deeptest.Config{
	Queue: database,
	Nodes: nodes,
	Tester: deepTester,
	BatchSize: 1,
	Concurrency: 1,
	Timeout: 20 * time.Second,
	Interval: 3 * time.Second,
})
go deepWorker.Run(ctx)
```

- [ ] **Step 2: Run command package tests**

Run:

```bash
go test ./cmd/region-proxy-gateway
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add cmd/region-proxy-gateway/main.go
git commit -m "feat: start deep test worker"
```

---

## Task 7: Channel History And 24-Hour Avoidance

**Files:**
- Modify: `internal/channel/manager.go`
- Modify: `internal/channel/manager_test.go`
- Modify: `cmd/region-proxy-gateway/main.go`

- [ ] **Step 1: Add failing channel tests**

Add to `internal/channel/manager_test.go`:

```go
func TestManagerRotationAvoidsRecentlyUsedNode(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{
		{ID: "jp-current", Region: "jp", IP: "203.0.113.1", LatencyMS: 10, Speed: 1000, Available: true},
		{ID: "jp-recent", Region: "jp", IP: "203.0.113.2", LatencyMS: 20, Speed: 900, Available: true},
		{ID: "jp-fresh", Region: "jp", IP: "203.0.113.3", LatencyMS: 30, Speed: 800, Available: true},
	})
	history := &fakeHistory{
		recent: map[string]time.Time{"jp-recent": time.Now().Add(-time.Hour)},
	}
	factory := &recordingFactory{}
	manager := NewManager(Config{
		Channels: []config.Channel{{ID: "jp-3000", ListenHost: "127.0.0.1", ListenPort: 3000, Region: "jp", RotateMinutes: 10, SelectionMode: config.SelectionAuto, Enabled: true}},
		Nodes: nodes,
		TunnelFactory: factory.New,
		History: history,
		DataDir: t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	if err := manager.RotateNow(context.Background(), "jp-3000"); err != nil {
		t.Fatalf("RotateNow: %v", err)
	}
	snapshot, _ := manager.Snapshot("jp-3000")
	if snapshot.CurrentNodeID != "jp-fresh" {
		t.Fatalf("current node = %q, want jp-fresh", snapshot.CurrentNodeID)
	}
}

type fakeHistory struct {
	recent map[string]time.Time
	uses []ChannelNodeUse
}

func (h *fakeHistory) RecentNodeIDsForChannel(ctx context.Context, channelID string, since time.Time) (map[string]time.Time, error) {
	return h.recent, nil
}

func (h *fakeHistory) RecordChannelNodeUse(ctx context.Context, use ChannelNodeUse) error {
	h.uses = append(h.uses, use)
	return nil
}
```

- [ ] **Step 2: Run channel test to verify failure**

Run:

```bash
go test ./internal/channel -run TestManagerRotationAvoidsRecentlyUsedNode
```

Expected: FAIL because `Config.History` and `ChannelNodeUse` do not exist.

- [ ] **Step 3: Implement history provider and avoid rule**

Modify `internal/channel/manager.go`:

Add:

```go
type ChannelNodeUse struct {
	ChannelID string
	NodeID string
	ExitIP string
	ConnectedAt time.Time
	SwitchedAt time.Time
}

type History interface {
	RecentNodeIDsForChannel(context.Context, string, time.Time) (map[string]time.Time, error)
	RecordChannelNodeUse(context.Context, ChannelNodeUse) error
}
```

Add `History History` to `Config`.

Record usage after successful `startLocked`, `SwitchToNode`, and `rotateLocked`:

```go
m.recordUse(ctx, ch.ID, n, time.Now())
```

Add helper:

```go
func (m *Manager) recordUse(ctx context.Context, channelID string, n node.Node, at time.Time) {
	if m.cfg.History == nil || n.ID == "" {
		return
	}
	_ = m.cfg.History.RecordChannelNodeUse(ctx, ChannelNodeUse{
		ChannelID: channelID,
		NodeID: n.ID,
		ExitIP: firstNonEmpty(n.IP, n.Hostname),
		ConnectedAt: at,
		SwitchedAt: at,
	})
}
```

Pass channel ID into `bestCheckedNode` and filter recent IDs:

```go
recent := map[string]time.Time{}
if m.cfg.History != nil {
	loaded, err := m.cfg.History.RecentNodeIDsForChannel(ctx, channelID, time.Now().Add(-24*time.Hour))
	if err == nil {
		recent = loaded
	}
}
```

When iterating candidates, skip IDs in `recent` unless all candidates would be skipped. Implement fallback by first trying strict filtering and then retrying without the recent filter.

- [ ] **Step 4: Wire storage adapter in main**

In `cmd/region-proxy-gateway/main.go`, pass `History: database` to `channel.NewManager`.

Storage `ChannelNodeUse` type is in `internal/storage`, while channel expects its own type. Add adapter methods to `storage.Store` with matching method names and signatures by importing `internal/channel` would create a cycle if storage imports channel. Instead, define channel `History` methods using an anonymous struct-compatible type:

```go
type NodeUseRecorder interface {
	RecordChannelNodeUse(context.Context, string, string, string, time.Time, time.Time) error
}
```

If method signatures conflict, use a small adapter in `cmd/region-proxy-gateway/main.go`.

- [ ] **Step 5: Run channel tests**

Run:

```bash
go test ./internal/channel
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/channel/manager.go internal/channel/manager_test.go cmd/region-proxy-gateway/main.go
git commit -m "feat: avoid recently used channel nodes"
```

---

## Task 8: Refresh Nodes Before Rotation

**Files:**
- Modify: `internal/channel/manager.go`
- Modify: `internal/channel/manager_test.go`
- Modify: `cmd/region-proxy-gateway/main.go`

- [ ] **Step 1: Add failing rotation refresh test**

Add to `internal/channel/manager_test.go`:

```go
func TestManagerRotationRefreshesNodesBeforeSelection(t *testing.T) {
	nodes := node.NewStore()
	nodes.Replace([]node.Node{{ID: "jp-a", Region: "jp", Available: true}})
	refreshed := false
	manager := NewManager(Config{
		Channels: []config.Channel{{ID: "jp-3000", ListenHost: "127.0.0.1", ListenPort: 3000, Region: "jp", RotateMinutes: 10, SelectionMode: config.SelectionAuto, Enabled: true}},
		Nodes: nodes,
		TunnelFactory: (&recordingFactory{}).New,
		RefreshNodes: func(ctx context.Context) ([]node.Node, error) {
			refreshed = true
			return []node.Node{
				{ID: "jp-a", Region: "jp", Available: true},
				{ID: "jp-b", Region: "jp", Available: true, LatencyMS: 10},
			}, nil
		},
		DataDir: t.TempDir(),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop(context.Background())

	if err := manager.RotateNow(context.Background(), "jp-3000"); err != nil {
		t.Fatalf("RotateNow: %v", err)
	}
	if !refreshed {
		t.Fatalf("expected refresh before rotation")
	}
	snapshot, _ := manager.Snapshot("jp-3000")
	if snapshot.CurrentNodeID != "jp-b" {
		t.Fatalf("current node = %q, want refreshed jp-b", snapshot.CurrentNodeID)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run:

```bash
go test ./internal/channel -run TestManagerRotationRefreshesNodesBeforeSelection
```

Expected: FAIL because `RefreshNodes` does not exist.

- [ ] **Step 3: Implement refresh callback**

In `internal/channel/manager.go`, add to `Config`:

```go
RefreshNodes func(context.Context) ([]node.Node, error)
```

At the start of `rotateLocked`, before selecting a node:

```go
if m.cfg.RefreshNodes != nil {
	refreshed, err := m.cfg.RefreshNodes(ctx)
	if err == nil && len(refreshed) > 0 {
		m.cfg.Nodes.Replace(refreshed)
	}
}
```

Log is not available inside channel package; keep refresh failure non-fatal and store it in `ch.err` only if no replacement can be selected.

- [ ] **Step 4: Wire main refresh callback**

In `cmd/region-proxy-gateway/main.go`, pass a callback that calls `loadNodes(ctx, cfg)`, replaces SQLite cache with `database.ReplaceNodes`, and returns the nodes.

- [ ] **Step 5: Run channel and command tests**

Run:

```bash
go test ./internal/channel ./cmd/region-proxy-gateway
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/channel/manager.go internal/channel/manager_test.go cmd/region-proxy-gateway/main.go
git commit -m "feat: refresh nodes before rotation"
```

---

## Task 9: Channel History In Admin UI

**Files:**
- Modify: `internal/admin/server.go`
- Modify: `internal/admin/server_test.go`
- Modify: `internal/admin/static.go`

- [ ] **Step 1: Add failing admin test**

Add to `internal/admin/server_test.go`:

```go
func TestChannelsIncludeHistoryFields(t *testing.T) {
	nodes, manager := newAdminTestManager(t)
	store := openAdminTestStore(t)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	if err := store.RecordChannelNodeUse(context.Background(), storage.ChannelNodeUse{
		ChannelID: "jp-3000",
		NodeID: "jp-1",
		ExitIP: "203.0.113.10",
		ConnectedAt: now.Add(-time.Hour),
		SwitchedAt: now,
	}); err != nil {
		t.Fatalf("RecordChannelNodeUse: %v", err)
	}
	server := NewServer(manager, nodes, nil, WithStorage(store))

	req := httptest.NewRequest(http.MethodGet, "/admin/api/channels", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"connected_at"`) || !strings.Contains(rec.Body.String(), `203.0.113.10`) {
		t.Fatalf("channels response missing history fields: %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run:

```bash
go test ./internal/admin -run TestChannelsIncludeHistoryFields
```

Expected: FAIL because channel view lacks history fields.

- [ ] **Step 3: Add fields to channel view**

Modify `channelView` in `internal/admin/server.go`:

```go
ConnectedAt  time.Time `json:"connected_at"`
SwitchedAt   time.Time `json:"switched_at"`
ExitIP       string    `json:"exit_ip"`
```

In `channelViewList`, if storage is configured, call `CurrentChannelNodeUse` per channel and set these fields.

- [ ] **Step 4: Update static UI**

Modify `internal/admin/static.go` channel columns:

- Replace existing `出口 IP` render to use `record.exit_ip || current_node.ip || current_node.hostname`.
- Add a compact time line below current node showing `连接` and `换 IP`.

Add helper:

```js
formatTime(value) {
  if (!value || value.startsWith('0001-')) return '-';
  return dayjs(value).format('MM-DD HH:mm:ss');
}
```

- [ ] **Step 5: Run admin tests**

Run:

```bash
go test ./internal/admin
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/admin/server.go internal/admin/server_test.go internal/admin/static.go
git commit -m "feat: show channel node history"
```

---

## Task 10: Documentation And Full Verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update README**

Add a section under management panel docs:

```markdown
## 深度测试和轮换避让

UDP OpenVPN 节点不能只靠 ping 判断可用性。管理面板提供“深度测试当前列表”，会把当前筛选出的节点加入 SQLite 队列，由后台 worker 逐个启动临时 OpenVPN 连接测试。

默认只跑 1 个深测 worker，避免同时启动多个 OpenVPN 进程导致资源和路由混乱。测试结果会保存到 SQLite，页面会显示深测成功、失败原因、出口 IP 和连接耗时。

通道轮换会先刷新 VPNGate 节点，再避开当前通道 24 小时内使用过的节点。如果同地区节点都在避让窗口内，会选择最久没用过的节点兜底。
```

- [ ] **Step 2: Run full verification**

Run:

```bash
go test ./...
go build -o /tmp/region-proxy-gateway ./cmd/region-proxy-gateway
```

Expected: both commands exit 0.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: document deep testing"
```

- [ ] **Step 4: Push**

```bash
git push origin main
```

Expected: push succeeds.

---

## Self-Review

- Spec coverage: queue, worker, SQLite result cache, channel history, admin UI, rotation avoid, refresh before rotation, and docs are covered.
- Placeholder scan: no task uses TBD/TODO placeholders; implementation code is concrete.
- Type consistency: `deeptest.Job`, `deeptest.Result`, `storage.ChannelNodeUse`, and channel history adapter are named consistently, except Task 7 explicitly calls out the package-cycle issue and requires an adapter if needed.
