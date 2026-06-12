package node

import "time"

type Node struct {
	ID           string    `json:"id"`
	Region       string    `json:"region"`
	Country      string    `json:"country"`
	IP           string    `json:"ip"`
	Hostname     string    `json:"hostname"`
	Port         int       `json:"port"`
	Proto        string    `json:"proto"`
	OpenVPN      string    `json:"openvpn"`
	LatencyMS    int       `json:"latency_ms"`
	Speed        int64     `json:"speed"`
	Available    bool      `json:"available"`
	LastTestedAt time.Time `json:"last_tested_at"`
	FailReason   string    `json:"fail_reason"`
	Owner        string    `json:"owner"`
	ASN          string    `json:"asn"`
	ASName       string    `json:"as_name"`
	Location     string    `json:"location"`
	IPType       string    `json:"ip_type"`
	Quality      string    `json:"quality"`
	PurityScore  int       `json:"purity_score"`
	ProbeStatus  string    `json:"probe_status"`
	ProbeMessage string    `json:"probe_message"`
	ProbedAt     time.Time `json:"probed_at"`
}
