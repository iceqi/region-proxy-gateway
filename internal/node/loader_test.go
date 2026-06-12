package node

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFileReadsNodes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodes.json")
	raw := `[
		{
			"id": "jp-1",
			"region": "jp",
			"country": "Japan",
			"ip": "203.0.113.10",
			"hostname": "vpn-jp.example.net",
			"openvpn": "client\nremote vpn-jp.example.net 1194 udp\n"
		}
	]`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatalf("write nodes file: %v", err)
	}

	nodes, err := LoadFile(path, RequireOpenVPNConfig)
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("node count = %d, want 1", len(nodes))
	}
	if nodes[0].ID != "jp-1" || nodes[0].Region != "jp" || !nodes[0].Available {
		t.Fatalf("node = %+v, want loaded available jp node", nodes[0])
	}
}

func TestLoadFileRejectsDuplicateIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodes.json")
	raw := `[
		{"id":"jp-1","region":"jp","openvpn":"client\n"},
		{"id":"jp-1","region":"jp","openvpn":"client\n"}
	]`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatalf("write nodes file: %v", err)
	}

	if _, err := LoadFile(path, RequireOpenVPNConfig); err == nil {
		t.Fatalf("expected duplicate ID error")
	}
}

func TestLoadFileRejectsMissingOpenVPNWhenRequired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodes.json")
	raw := `[{"id":"jp-1","region":"jp"}]`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatalf("write nodes file: %v", err)
	}

	if _, err := LoadFile(path, RequireOpenVPNConfig); err == nil {
		t.Fatalf("expected missing OpenVPN config error")
	}
}

func TestLoadFileAllowsMissingOpenVPNWhenNotRequired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodes.json")
	raw := `[{"id":"jp-1","region":"jp"}]`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatalf("write nodes file: %v", err)
	}

	nodes, err := LoadFile(path, AllowMissingOpenVPNConfig)
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("node count = %d, want 1", len(nodes))
	}
}
