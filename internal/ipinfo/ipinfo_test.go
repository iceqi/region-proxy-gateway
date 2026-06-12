package ipinfo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iceqi/region-proxy-gateway/internal/node"
)

func TestClientEnrichesNodesWithIPTypeAndPurity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		var queries []string
		if err := json.NewDecoder(r.Body).Decode(&queries); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(queries) != 2 || queries[0] != "203.0.113.10" || queries[1] != "198.51.100.20" {
			t.Fatalf("queries = %+v", queries)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"status":     "success",
				"query":      "203.0.113.10",
				"country":    "日本",
				"regionName": "东京",
				"city":       "东京",
				"isp":        "Example Fiber",
				"org":        "Example Home",
				"as":         "AS64500 Example",
				"asname":     "EXAMPLE-HOME",
				"proxy":      false,
				"hosting":    false,
				"mobile":     false,
			},
			{
				"status":  "success",
				"query":   "198.51.100.20",
				"isp":     "Cloud Host",
				"org":     "Cloud Host",
				"proxy":   true,
				"hosting": true,
				"mobile":  false,
			},
		})
	}))
	defer server.Close()

	nodes := []node.Node{
		{ID: "jp-1", IP: "203.0.113.10"},
		{ID: "us-1", IP: "198.51.100.20"},
	}
	client := Client{URL: server.URL, HTTPClient: server.Client()}

	got, err := client.Enrich(context.Background(), nodes)
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}

	if got[0].IPType != "residential" {
		t.Fatalf("ip type = %q, want residential", got[0].IPType)
	}
	if got[0].Quality != "normal" {
		t.Fatalf("quality = %q, want normal", got[0].Quality)
	}
	if got[0].PurityScore != 90 {
		t.Fatalf("purity = %d, want 90", got[0].PurityScore)
	}
	if got[0].Owner != "Example Home" || got[0].ASN != "AS64500 Example" || got[0].ASName != "EXAMPLE-HOME" {
		t.Fatalf("owner/as fields not enriched: %+v", got[0])
	}
	if got[0].Location != "日本 东京 东京" {
		t.Fatalf("location = %q, want 日本 东京 东京", got[0].Location)
	}
	if got[1].IPType != "proxy" || got[1].Quality != "proxy" || got[1].PurityScore != 20 {
		t.Fatalf("proxy node not classified: %+v", got[1])
	}
}

func TestClientSkipsNodesWithoutIP(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	client := Client{URL: server.URL, HTTPClient: server.Client()}
	got, err := client.Enrich(context.Background(), []node.Node{{ID: "no-ip"}})
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	if called {
		t.Fatalf("server should not be called without IPs")
	}
	if len(got) != 1 || got[0].ID != "no-ip" {
		t.Fatalf("nodes changed unexpectedly: %+v", got)
	}
}
