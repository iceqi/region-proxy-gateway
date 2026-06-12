package vpngate

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestParseCSVDecodesOpenVPNConfigAndSortsNodes(t *testing.T) {
	fastConfig := base64.StdEncoding.EncodeToString([]byte("client\nremote fast.example 1194 udp\n"))
	slowConfig := base64.StdEncoding.EncodeToString([]byte("client\nremote slow.example 1194 udp\n"))
	body := strings.Join([]string{
		"*vpn_servers",
		"#HostName,IP,Score,Ping,Speed,CountryLong,CountryShort,NumVpnSessions,Uptime,TotalUsers,TotalTraffic,LogType,Operator,Message,OpenVPN_ConfigData_Base64",
		"slow.example,198.51.100.2,10,80,1000,Japan,JP,1,1,1,1,2,,, " + slowConfig,
		"fast.example,198.51.100.1,10,30,9000,Japan,JP,1,1,1,1,2,,," + fastConfig,
		"*",
	}, "\n")

	nodes, err := ParseCSV(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(nodes))
	}
	if nodes[0].Hostname != "fast.example" {
		t.Fatalf("first host = %q, want fast.example", nodes[0].Hostname)
	}
	if nodes[0].Region != "jp" {
		t.Fatalf("region = %q, want jp", nodes[0].Region)
	}
	if nodes[0].OpenVPN == "" || !strings.Contains(nodes[0].OpenVPN, "remote fast.example") {
		t.Fatalf("decoded openvpn config missing fast remote: %q", nodes[0].OpenVPN)
	}
	if nodes[0].Port != 1194 {
		t.Fatalf("port = %d, want 1194", nodes[0].Port)
	}
	if nodes[0].Proto != "udp" {
		t.Fatalf("proto = %q, want udp", nodes[0].Proto)
	}
	if !nodes[0].Available {
		t.Fatalf("node should be available")
	}
}

func TestParseCSVSkipsRowsWithoutOpenVPNConfig(t *testing.T) {
	body := strings.Join([]string{
		"#HostName,IP,Ping,Speed,CountryLong,CountryShort,OpenVPN_ConfigData_Base64",
		"empty.example,198.51.100.3,10,1000,Japan,JP,",
		"*",
	}, "\n")

	nodes, err := ParseCSV(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(nodes) != 0 {
		t.Fatalf("nodes = %d, want 0", len(nodes))
	}
}
