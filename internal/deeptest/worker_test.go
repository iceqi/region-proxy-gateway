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
