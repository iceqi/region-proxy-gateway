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
