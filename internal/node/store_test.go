package node

import (
	"testing"
	"time"
)

func TestBestByRegionReturnsLowestLatencyAvailableNode(t *testing.T) {
	store := NewStore()
	store.Replace([]Node{
		{ID: "jp-slow", Region: "jp", LatencyMS: 80, Available: true},
		{ID: "jp-fast", Region: "jp", LatencyMS: 20, Available: true},
		{ID: "us-fast", Region: "us", LatencyMS: 10, Available: true},
	})

	got, ok := store.BestByRegion("jp", "")
	if !ok {
		t.Fatal("expected a jp node")
	}
	if got.ID != "jp-fast" {
		t.Fatalf("expected jp-fast, got %q", got.ID)
	}
}

func TestBestByRegionPrefersMeasuredLatencyOverCSVSpeed(t *testing.T) {
	store := NewStore()
	store.Replace([]Node{
		{ID: "jp-low-latency", Region: "jp", LatencyMS: 20, Speed: 100, Available: true},
		{ID: "jp-high-speed", Region: "jp", LatencyMS: 80, Speed: 9000, Available: true},
	})

	got, ok := store.BestByRegion("jp", "")
	if !ok {
		t.Fatal("expected a jp node")
	}
	if got.ID != "jp-low-latency" {
		t.Fatalf("expected jp-low-latency, got %q", got.ID)
	}
}

func TestCandidatesByRegionReturnsSortedCopy(t *testing.T) {
	store := NewStore()
	store.Replace([]Node{
		{ID: "jp-slow", Region: "jp", LatencyMS: 80, Available: true},
		{ID: "jp-fast", Region: "jp", LatencyMS: 20, Available: true},
		{ID: "jp-current", Region: "jp", LatencyMS: 10, Available: true},
		{ID: "us-fast", Region: "us", LatencyMS: 10, Available: true},
	})

	got := store.CandidatesByRegion("jp", "jp-current", 1)
	if len(got) != 1 || got[0].ID != "jp-fast" {
		t.Fatalf("candidates = %+v, want only jp-fast", got)
	}
	got[0].ID = "mutated"
	again := store.CandidatesByRegion("jp", "jp-current", 1)
	if again[0].ID != "jp-fast" {
		t.Fatalf("candidates should be copied, got %+v", again)
	}
}

func TestNodeByIDReturnsCopy(t *testing.T) {
	store := NewStore()
	store.Replace([]Node{{ID: "jp-1", Region: "jp", Available: true}})

	got, ok := store.NodeByID("jp-1")
	if !ok {
		t.Fatalf("NodeByID ok = false, want true")
	}
	got.Region = "us"

	again, ok := store.NodeByID("jp-1")
	if !ok || again.Region != "jp" {
		t.Fatalf("NodeByID returned mutable data: %+v ok=%v", again, ok)
	}
}

func TestBestByRegionAvoidsPreviousNodeWhenAlternativeExists(t *testing.T) {
	store := NewStore()
	store.Replace([]Node{
		{ID: "jp-fast", Region: "jp", LatencyMS: 20, Available: true},
		{ID: "jp-backup", Region: "jp", LatencyMS: 30, Available: true},
	})

	got, ok := store.BestByRegion("jp", "jp-fast")
	if !ok {
		t.Fatal("expected a jp node")
	}
	if got.ID != "jp-backup" {
		t.Fatalf("expected jp-backup, got %q", got.ID)
	}
}

func TestBestByRegionIgnoresUnavailableNodes(t *testing.T) {
	store := NewStore()
	store.Replace([]Node{
		{ID: "jp-unavailable", Region: "jp", LatencyMS: 10, Available: false},
		{ID: "jp-available", Region: "jp", LatencyMS: 40, Available: true},
	})

	got, ok := store.BestByRegion("jp", "")
	if !ok {
		t.Fatal("expected a jp node")
	}
	if got.ID != "jp-available" {
		t.Fatalf("expected jp-available, got %q", got.ID)
	}
}

func TestStoreCopiesNodeSlices(t *testing.T) {
	store := NewStore()
	input := []Node{
		{ID: "jp-1", Region: "jp", LatencyMS: 20, Available: true, LastTestedAt: time.Unix(1, 0)},
	}
	store.Replace(input)

	input[0].ID = "mutated"
	list := store.List()
	if list[0].ID != "jp-1" {
		t.Fatalf("expected Replace to copy input slice, got %q", list[0].ID)
	}

	list[0].ID = "changed"
	listAgain := store.List()
	if listAgain[0].ID != "jp-1" {
		t.Fatalf("expected List to return a copy, got %q", listAgain[0].ID)
	}
}

func TestBestByRegionReturnsFalseWhenNoAvailableNode(t *testing.T) {
	store := NewStore()
	store.Replace([]Node{
		{ID: "jp-unavailable", Region: "jp", LatencyMS: 10, Available: false},
		{ID: "us-available", Region: "us", LatencyMS: 10, Available: true},
	})

	got, ok := store.BestByRegion("jp", "")
	if ok {
		t.Fatalf("expected no available jp node, got %+v", got)
	}
}

func TestUpdateNodeReplacesMatchingNode(t *testing.T) {
	store := NewStore()
	store.Replace([]Node{
		{ID: "jp-1", Region: "jp", LatencyMS: 100, Available: true},
		{ID: "us-1", Region: "us", LatencyMS: 200, Available: true},
	})

	ok := store.Update("jp-1", func(n Node) Node {
		n.LatencyMS = 25
		n.FailReason = ""
		return n
	})

	if !ok {
		t.Fatalf("expected update to find node")
	}
	nodes := store.List()
	if nodes[0].LatencyMS != 25 {
		t.Fatalf("latency = %d, want 25", nodes[0].LatencyMS)
	}
	if nodes[1].LatencyMS != 200 {
		t.Fatalf("other node latency = %d, want 200", nodes[1].LatencyMS)
	}
}
