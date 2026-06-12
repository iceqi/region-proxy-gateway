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

type QueueStats struct {
	Pending int `json:"pending"`
	Running int `json:"running"`
	Success int `json:"success"`
	Failed  int `json:"failed"`
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
