package vpngate

import (
	"context"
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/node"
)

const APIURL = "https://www.vpngate.net/api/iphone/"

type Client struct {
	HTTPClient *http.Client
	URL        string
}

func (c Client) Fetch(ctx context.Context) ([]node.Node, error) {
	url := c.URL
	if url == "" {
		url = APIURL
	}
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("vpngate returned %s", resp.Status)
	}
	return ParseCSV(resp.Body)
}

func ParseCSV(r io.Reader) ([]node.Node, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(raw), "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "*" || strings.HasPrefix(line, "*vpn_servers") {
			continue
		}
		if strings.HasPrefix(line, "#") {
			line = strings.TrimPrefix(line, "#")
		}
		cleaned = append(cleaned, line)
	}
	if len(cleaned) == 0 {
		return nil, nil
	}

	reader := csv.NewReader(strings.NewReader(strings.Join(cleaned, "\n")))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse vpngate csv: %w", err)
	}
	if len(records) < 2 {
		return nil, nil
	}

	header := map[string]int{}
	for i, name := range records[0] {
		header[strings.TrimSpace(name)] = i
	}

	nodes := make([]node.Node, 0, len(records)-1)
	for _, record := range records[1:] {
		openVPNBase64 := csvValue(record, header, "OpenVPN_ConfigData_Base64")
		if openVPNBase64 == "" {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(openVPNBase64))
		if err != nil || len(decoded) == 0 {
			continue
		}
		region := strings.ToLower(csvValue(record, header, "CountryShort"))
		host := csvValue(record, header, "HostName")
		ip := csvValue(record, header, "IP")
		if region == "" || (host == "" && ip == "") {
			continue
		}

		idSource := firstNonEmpty(host, ip)
		remoteHost, remotePort, remoteProto := parseOpenVPNRemote(string(decoded))
		if host == "" {
			host = remoteHost
		}
		nodes = append(nodes, node.Node{
			ID:        region + "-" + idSource,
			Region:    region,
			Country:   csvValue(record, header, "CountryLong"),
			IP:        ip,
			Hostname:  host,
			Port:      remotePort,
			Proto:     remoteProto,
			OpenVPN:   string(decoded),
			LatencyMS: parseInt(csvValue(record, header, "Ping")),
			Speed:     parseInt64(csvValue(record, header, "Speed")),
			Available: true,
		})
	}

	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].Speed != nodes[j].Speed {
			return nodes[i].Speed > nodes[j].Speed
		}
		if nodes[i].LatencyMS == 0 {
			return false
		}
		if nodes[j].LatencyMS == 0 {
			return true
		}
		return nodes[i].LatencyMS < nodes[j].LatencyMS
	})
	return nodes, nil
}

func csvValue(record []string, header map[string]int, name string) string {
	index, ok := header[name]
	if !ok || index < 0 || index >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[index])
}

func parseInt(value string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(value))
	return n
}

func parseInt64(value string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return n
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func parseOpenVPNRemote(config string) (string, int, string) {
	for _, line := range strings.Split(config, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 || fields[0] != "remote" {
			continue
		}
		host := fields[1]
		port := 1194
		proto := "udp"
		if len(fields) >= 3 {
			if parsed := parseInt(fields[2]); parsed > 0 {
				port = parsed
			}
		}
		if len(fields) >= 4 {
			proto = strings.ToLower(fields[3])
		}
		return host, port, proto
	}
	return "", 1194, "udp"
}
