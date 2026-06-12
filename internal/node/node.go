package node

import "time"

type Node struct {
	ID           string    `json:"id"`
	Region       string    `json:"region"`
	Country      string    `json:"country"`
	IP           string    `json:"ip"`
	Hostname     string    `json:"hostname"`
	OpenVPN      string    `json:"openvpn"`
	LatencyMS    int       `json:"latency_ms"`
	Available    bool      `json:"available"`
	LastTestedAt time.Time `json:"last_tested_at"`
	FailReason   string    `json:"fail_reason"`
}
