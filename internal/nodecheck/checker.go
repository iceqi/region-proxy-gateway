package nodecheck

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/iceqi/region-proxy-gateway/internal/node"
)

type PingFunc func(context.Context, string, time.Duration) (int, error)

type Checker struct {
	Timeout time.Duration
	Ping    PingFunc
}

func (c Checker) Check(ctx context.Context, n node.Node) node.Node {
	if c.Timeout == 0 {
		c.Timeout = 3 * time.Second
	}
	host := n.IP
	if host == "" {
		host = n.Hostname
	}
	if host == "" {
		n.Available = false
		n.ProbeStatus = "unavailable"
		n.ProbeMessage = "missing host"
		n.FailReason = n.ProbeMessage
		n.ProbedAt = time.Now()
		n.LastTestedAt = n.ProbedAt
		return n
	}

	proto := strings.ToLower(n.Proto)
	if proto == "" {
		proto = "udp"
	}
	if proto == "tcp" || strings.HasPrefix(proto, "tcp") {
		latency, err := tcpConnectLatency(ctx, host, n.Port, c.Timeout)
		if err != nil {
			n.Available = false
			n.ProbeStatus = "unavailable"
			n.ProbeMessage = "tcp connect failed: " + err.Error()
			n.FailReason = n.ProbeMessage
			n.ProbedAt = time.Now()
			n.LastTestedAt = n.ProbedAt
			return n
		}
		n.LatencyMS = latency
		n.Available = true
		n.FailReason = ""
		n.ProbeStatus = "available"
		n.ProbeMessage = "tcp port ok"
		n.ProbedAt = time.Now()
		n.LastTestedAt = n.ProbedAt
		return n
	}

	ping := c.Ping
	if ping == nil {
		ping = SystemPing
	}
	latency, pingErr := ping(ctx, host, c.Timeout)
	if pingErr != nil {
		n.Available = true
		n.ProbeStatus = "unknown"
		n.ProbeMessage = "udp host unreachable; deprioritized until deep test or successful ping"
		n.FailReason = ""
		n.ProbedAt = time.Now()
		n.LastTestedAt = n.ProbedAt
		return n
	}
	n.LatencyMS = latency
	n.Available = true
	n.FailReason = ""
	n.ProbeStatus = "available"
	n.ProbeMessage = "ping ok; udp port cannot be fully verified without vpn handshake"
	n.ProbedAt = time.Now()
	n.LastTestedAt = n.ProbedAt
	return n
}

func tcpConnectLatency(ctx context.Context, host string, port int, timeout time.Duration) (int, error) {
	if port == 0 {
		port = 1194
	}
	target := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	dialer := net.Dialer{Timeout: timeout}
	started := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		return 0, err
	}
	elapsed := int(time.Since(started).Milliseconds())
	if elapsed < 1 {
		elapsed = 1
	}
	return elapsed, conn.Close()
}

var pingTimePattern = regexp.MustCompile(`time[=<]([0-9.]+)\s*ms`)

func SystemPing(ctx context.Context, host string, timeout time.Duration) (int, error) {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{"-c", "1"}
	if runtime.GOOS == "darwin" {
		args = append(args, "-W", strconv.Itoa(int(timeout.Milliseconds())))
	} else {
		seconds := int(timeout.Seconds())
		if seconds < 1 {
			seconds = 1
		}
		args = append(args, "-W", strconv.Itoa(seconds))
	}
	args = append(args, host)
	cmd := exec.CommandContext(ctx, "ping", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	started := time.Now()
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("%w: %s", err, strings.TrimSpace(out.String()))
	}
	matches := pingTimePattern.FindStringSubmatch(out.String())
	if len(matches) >= 2 {
		ms, err := strconv.ParseFloat(matches[1], 64)
		if err == nil {
			if ms < 1 {
				return 1, nil
			}
			return int(ms + 0.5), nil
		}
	}
	elapsed := int(time.Since(started).Milliseconds())
	if elapsed < 1 {
		elapsed = 1
	}
	return elapsed, nil
}
