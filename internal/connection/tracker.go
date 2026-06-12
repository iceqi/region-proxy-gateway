package connection

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type Record struct {
	ID         string    `json:"id"`
	ClientAddr string    `json:"client_addr"`
	Protocol   string    `json:"protocol"`
	ChannelID  string    `json:"channel_id"`
	Target     string    `json:"target"`
	StartedAt  time.Time `json:"started_at"`
	BytesUp    int64     `json:"bytes_up"`
	BytesDown  int64     `json:"bytes_down"`
}

type Tracker struct {
	counter atomic.Uint64
	mu      sync.RWMutex
	records map[string]Record
}

func NewTracker() *Tracker {
	return &Tracker{
		records: make(map[string]Record),
	}
}

func (t *Tracker) Start(clientAddr, protocol, channelID, target string) string {
	n := t.counter.Add(1)
	id := fmt.Sprintf("conn-%d", n)

	t.mu.Lock()
	defer t.mu.Unlock()
	t.records[id] = Record{
		ID:         id,
		ClientAddr: clientAddr,
		Protocol:   protocol,
		ChannelID:  channelID,
		Target:     target,
		StartedAt:  time.Now(),
	}
	return id
}

func (t *Tracker) AddBytes(id string, up, down int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	record, ok := t.records[id]
	if !ok {
		return
	}
	record.BytesUp += up
	record.BytesDown += down
	t.records[id] = record
}

func (t *Tracker) Finish(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.records, id)
}

func (t *Tracker) List() []Record {
	t.mu.RLock()
	defer t.mu.RUnlock()

	records := make([]Record, 0, len(t.records))
	for _, record := range t.records {
		records = append(records, record)
	}
	return records
}

func (t *Tracker) ActiveCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.records)
}
