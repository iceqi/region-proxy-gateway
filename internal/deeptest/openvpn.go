package deeptest

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/node"
	"github.com/iceqi/region-proxy-gateway/internal/tunnel"
)

type OpenVPNTester struct {
	DataDir       string
	Command       string
	Starter       tunnel.OpenVPNProcessStarter
	DeviceDialer  tunnel.DeviceDialer
	HTTPClient    *http.Client
	ExitIPURL     string
	RetryInterval time.Duration
}

func (t OpenVPNTester) Test(ctx context.Context, n node.Node) Result {
	started := time.Now()
	result := Result{NodeID: n.ID, Status: StatusFailed, TestedAt: started}
	if strings.TrimSpace(n.OpenVPN) == "" {
		result.FailReason = "empty openvpn config"
		return result
	}

	deviceName := testDeviceName(n.ID)
	sessionName := "deeptest-" + sanitizeName(n.ID)
	vpn := tunnel.NewOpenVPN(tunnel.OpenVPNConfig{
		DataDir:      t.DataDir,
		Command:      t.Command,
		Starter:      t.Starter,
		DeviceDialer: firstDeviceDialer(t.DeviceDialer),
		StopTimeout:  3 * time.Second,
	})
	if err := vpn.Start(ctx, n, tunnel.Options{Name: sessionName, DataDir: t.DataDir, Command: t.Command, DeviceName: deviceName}); err != nil {
		result.FailReason = err.Error()
		return result
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = vpn.Stop(stopCtx)
	}()

	exit, err := t.waitForExit(ctx, deviceName)
	if err != nil {
		result.FailReason = "exit IP check failed: " + err.Error()
		return result
	}
	result.Status = StatusSuccess
	result.ExitIP = exit.IP
	result.ExitCountry = exit.Country
	result.ConnectMS = int(time.Since(started).Milliseconds())
	if result.ConnectMS <= 0 {
		result.ConnectMS = 1
	}
	result.TestedAt = time.Now()
	return result
}

func (t OpenVPNTester) waitForExit(ctx context.Context, deviceName string) (exitIPResponse, error) {
	interval := t.RetryInterval
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	var lastErr error
	for {
		exit, err := t.lookupExit(ctx, deviceName)
		if err == nil {
			return exit, nil
		}
		lastErr = err
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			if lastErr != nil {
				return exitIPResponse{}, lastErr
			}
			return exitIPResponse{}, ctx.Err()
		case <-timer.C:
		}
	}
}

type exitIPResponse struct {
	IP      string `json:"ip"`
	Query   string `json:"query"`
	Country string `json:"country"`
}

func (t OpenVPNTester) lookupExit(ctx context.Context, deviceName string) (exitIPResponse, error) {
	url := strings.TrimSpace(t.ExitIPURL)
	if url == "" {
		url = "https://ipinfo.io/json"
	}
	client := t.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := firstDeviceDialer(t.DeviceDialer)
	baseTransport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return dialer.DialContext(ctx, deviceName, network, address)
	}
	httpClient := *client
	httpClient.Transport = baseTransport

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return exitIPResponse{}, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return exitIPResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return exitIPResponse{}, fmt.Errorf("exit ip endpoint returned %s", resp.Status)
	}
	var out exitIPResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return exitIPResponse{}, err
	}
	if out.IP == "" {
		out.IP = out.Query
	}
	if out.IP == "" {
		return exitIPResponse{}, fmt.Errorf("exit ip endpoint returned empty ip")
	}
	return out, nil
}

func firstDeviceDialer(dialer tunnel.DeviceDialer) tunnel.DeviceDialer {
	if dialer != nil {
		return dialer
	}
	return tunnel.SystemDeviceDialer{}
}

func testDeviceName(nodeID string) string {
	return fmt.Sprintf("rpt%08x", crc32.ChecksumIEEE([]byte(nodeID)))
}

func sanitizeName(value string) string {
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "node"
	}
	return b.String()
}
