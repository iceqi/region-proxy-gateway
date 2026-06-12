package deeptest

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/node"
)

type Queue interface {
	ClaimDeepTestJobs(ctx context.Context, limit int, now time.Time) ([]Job, error)
	CompleteDeepTestJob(ctx context.Context, jobID int64, result Result) error
}

type NodeSource interface {
	NodeByID(id string) (node.Node, bool)
}

type Tester interface {
	Test(ctx context.Context, n node.Node) Result
}

type TesterFunc func(ctx context.Context, n node.Node) Result

func (f TesterFunc) Test(ctx context.Context, n node.Node) Result {
	return f(ctx, n)
}

type Config struct {
	Queue       Queue
	Nodes       NodeSource
	Tester      Tester
	BatchSize   int
	Concurrency int
	Interval    time.Duration
	Timeout     time.Duration
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
	if cfg.Interval <= 0 {
		cfg.Interval = 3 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 20 * time.Second
	}
	return &Worker{cfg: cfg}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.cfg.Interval)
	defer ticker.Stop()
	for {
		_, _ = w.RunOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	if w.cfg.Queue == nil {
		return 0, fmt.Errorf("deep test queue is required")
	}
	if w.cfg.Nodes == nil {
		return 0, fmt.Errorf("deep test node source is required")
	}
	if w.cfg.Tester == nil {
		return 0, fmt.Errorf("deep test tester is required")
	}
	jobs, err := w.cfg.Queue.ClaimDeepTestJobs(ctx, w.cfg.BatchSize, time.Now())
	if err != nil {
		return 0, err
	}
	if len(jobs) == 0 {
		return 0, nil
	}

	sem := make(chan struct{}, w.cfg.Concurrency)
	var wg sync.WaitGroup
	for _, job := range jobs {
		job := job
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() {
				<-sem
				wg.Done()
			}()
			result := w.runJob(ctx, job)
			_ = w.cfg.Queue.CompleteDeepTestJob(context.Background(), job.ID, result)
		}()
	}
	wg.Wait()
	return len(jobs), nil
}

func (w *Worker) runJob(ctx context.Context, job Job) (result Result) {
	result = Result{
		NodeID:     job.NodeID,
		Status:     StatusFailed,
		TestedAt:   time.Now(),
		FailReason: "unknown failure",
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			result = Result{
				NodeID:     job.NodeID,
				Status:     StatusFailed,
				TestedAt:   time.Now(),
				FailReason: fmt.Sprintf("tester panic: %v", recovered),
			}
		}
		if result.NodeID == "" {
			result.NodeID = job.NodeID
		}
		if result.Status == "" {
			result.Status = StatusFailed
		}
		if result.TestedAt.IsZero() {
			result.TestedAt = time.Now()
		}
	}()

	n, ok := w.cfg.Nodes.NodeByID(job.NodeID)
	if !ok {
		result.FailReason = "node not found"
		return result
	}
	jobCtx, cancel := context.WithTimeout(ctx, w.cfg.Timeout)
	defer cancel()
	return w.cfg.Tester.Test(jobCtx, n)
}
