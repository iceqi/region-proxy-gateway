package connection

import (
	"sync"
	"testing"
)

func TestStartIncrementsActiveCountAndReturnsRecord(t *testing.T) {
	tracker := NewTracker()

	id := tracker.Start("127.0.0.1:54321", "http", "jp-3000", "example.com:443")

	if id != "conn-1" {
		t.Fatalf("id = %q, want conn-1", id)
	}
	if tracker.ActiveCount() != 1 {
		t.Fatalf("active count = %d, want 1", tracker.ActiveCount())
	}
	records := tracker.List()
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	record := records[0]
	if record.ID != id {
		t.Fatalf("record id = %q, want %q", record.ID, id)
	}
	if record.ClientAddr != "127.0.0.1:54321" {
		t.Fatalf("client addr = %q, want 127.0.0.1:54321", record.ClientAddr)
	}
	if record.Protocol != "http" {
		t.Fatalf("protocol = %q, want http", record.Protocol)
	}
	if record.ChannelID != "jp-3000" {
		t.Fatalf("channel id = %q, want jp-3000", record.ChannelID)
	}
	if record.Target != "example.com:443" {
		t.Fatalf("target = %q, want example.com:443", record.Target)
	}
	if record.StartedAt.IsZero() {
		t.Fatal("expected started_at to be set")
	}
}

func TestAddBytesUpdatesUploadAndDownloadCounters(t *testing.T) {
	tracker := NewTracker()
	id := tracker.Start("client", "socks5", "us-0", "target")

	tracker.AddBytes(id, 100, 250)
	tracker.AddBytes(id, 5, 10)
	tracker.AddBytes("missing", 99, 99)

	records := tracker.List()
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	if records[0].BytesUp != 105 {
		t.Fatalf("bytes up = %d, want 105", records[0].BytesUp)
	}
	if records[0].BytesDown != 260 {
		t.Fatalf("bytes down = %d, want 260", records[0].BytesDown)
	}
}

func TestFinishRemovesConnection(t *testing.T) {
	tracker := NewTracker()
	id := tracker.Start("client", "http", "jp-15", "target")

	tracker.Finish(id)
	tracker.Finish("missing")

	if tracker.ActiveCount() != 0 {
		t.Fatalf("active count = %d, want 0", tracker.ActiveCount())
	}
	if records := tracker.List(); len(records) != 0 {
		t.Fatalf("record count = %d, want 0", len(records))
	}
}

func TestListReturnsCopy(t *testing.T) {
	tracker := NewTracker()
	id := tracker.Start("client", "http", "jp-15", "target")

	records := tracker.List()
	records[0].ID = "mutated"
	records[0].BytesUp = 999

	recordsAgain := tracker.List()
	if recordsAgain[0].ID != id {
		t.Fatalf("record id = %q, want %q", recordsAgain[0].ID, id)
	}
	if recordsAgain[0].BytesUp != 0 {
		t.Fatalf("bytes up = %d, want 0", recordsAgain[0].BytesUp)
	}
}

func TestStartGeneratesUniqueConcurrentIDs(t *testing.T) {
	tracker := NewTracker()
	const total = 100
	var wg sync.WaitGroup
	ids := make(chan string, total)

	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids <- tracker.Start("client", "http", "jp-15", "target")
		}()
	}
	wg.Wait()
	close(ids)

	seen := map[string]bool{}
	for id := range ids {
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
	if len(seen) != total {
		t.Fatalf("id count = %d, want %d", len(seen), total)
	}
	if tracker.ActiveCount() != total {
		t.Fatalf("active count = %d, want %d", tracker.ActiveCount(), total)
	}
}
