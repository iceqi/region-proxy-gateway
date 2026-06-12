package node

import (
	"encoding/json"
	"fmt"
	"os"
)

type OpenVPNRequirement bool

const (
	AllowMissingOpenVPNConfig OpenVPNRequirement = false
	RequireOpenVPNConfig      OpenVPNRequirement = true
)

func LoadFile(path string, requireOpenVPN OpenVPNRequirement) ([]Node, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read nodes file %q: %w", path, err)
	}

	var nodes []Node
	if err := json.Unmarshal(raw, &nodes); err != nil {
		return nil, fmt.Errorf("parse nodes file %q: %w", path, err)
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("nodes file %q contains no nodes", path)
	}

	seen := map[string]struct{}{}
	for i := range nodes {
		if nodes[i].ID == "" {
			return nil, fmt.Errorf("node at index %d has empty id", i)
		}
		if nodes[i].Region == "" {
			return nil, fmt.Errorf("node %q has empty region", nodes[i].ID)
		}
		if requireOpenVPN && nodes[i].OpenVPN == "" {
			return nil, fmt.Errorf("node %q has empty openvpn config", nodes[i].ID)
		}
		if _, ok := seen[nodes[i].ID]; ok {
			return nil, fmt.Errorf("duplicate node id %q", nodes[i].ID)
		}
		seen[nodes[i].ID] = struct{}{}
		nodes[i].Available = true
	}

	return nodes, nil
}
