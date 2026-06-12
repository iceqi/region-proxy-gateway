package ipinfo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/node"
)

const DefaultURL = "http://ip-api.com/batch?lang=zh-CN&fields=status,message,query,country,regionName,city,isp,org,as,asname,proxy,hosting,mobile"

type Client struct {
	URL        string
	HTTPClient *http.Client
}

type responseItem struct {
	Status     string `json:"status"`
	Message    string `json:"message"`
	Query      string `json:"query"`
	Country    string `json:"country"`
	RegionName string `json:"regionName"`
	City       string `json:"city"`
	ISP        string `json:"isp"`
	Org        string `json:"org"`
	AS         string `json:"as"`
	ASName     string `json:"asname"`
	Proxy      bool   `json:"proxy"`
	Hosting    bool   `json:"hosting"`
	Mobile     bool   `json:"mobile"`
}

func (c Client) Enrich(ctx context.Context, nodes []node.Node) ([]node.Node, error) {
	out := append([]node.Node(nil), nodes...)
	ips := make([]string, 0, len(out))
	seen := map[string]struct{}{}
	for _, n := range out {
		ip := strings.TrimSpace(n.IP)
		if ip == "" {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		ips = append(ips, ip)
	}
	if len(ips) == 0 {
		return out, nil
	}

	info, err := c.lookup(ctx, ips)
	if err != nil {
		return out, err
	}
	for i := range out {
		item, ok := info[out[i].IP]
		if !ok {
			continue
		}
		apply(&out[i], item)
	}
	return out, nil
}

func (c Client) lookup(ctx context.Context, ips []string) (map[string]responseItem, error) {
	url := c.URL
	if url == "" {
		url = DefaultURL
	}
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	raw, err := json.Marshal(ips)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ip info returned %s", resp.Status)
	}
	var items []responseItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}
	byIP := make(map[string]responseItem, len(items))
	for _, item := range items {
		if item.Status != "success" || item.Query == "" {
			continue
		}
		byIP[item.Query] = item
	}
	return byIP, nil
}

func apply(n *node.Node, item responseItem) {
	n.Owner = firstNonEmpty(item.Org, item.ISP)
	n.ASN = item.AS
	n.ASName = item.ASName
	n.Location = joinNonEmpty(" ", item.Country, item.RegionName, item.City)

	n.IPType = "residential"
	n.Quality = "normal"
	n.PurityScore = 90
	if item.Proxy {
		n.IPType = "proxy"
		n.Quality = "proxy"
		n.PurityScore = 20
		return
	}
	if item.Hosting {
		n.IPType = "hosting"
		n.Quality = "datacenter"
		n.PurityScore = 45
		return
	}
	if item.Mobile {
		n.IPType = "mobile"
		n.Quality = "mobile"
		n.PurityScore = 80
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func joinNonEmpty(sep string, values ...string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, sep)
}
