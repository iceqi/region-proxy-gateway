package config

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
)

const (
	TunnelBackendFake    = "fake"
	TunnelBackendOpenVPN = "openvpn"

	SelectionAuto   = "auto"
	SelectionManual = "manual"
)

type Config struct {
	AdminHost           string    `json:"admin_host"`
	AdminPort           int       `json:"admin_port"`
	AdminPath           string    `json:"admin_path"`
	AdminUsername       string    `json:"admin_username"`
	AdminPassword       string    `json:"admin_password"`
	ProxyUsername       string    `json:"proxy_username"`
	ProxyPassword       string    `json:"proxy_password"`
	NodeRefreshInterval string    `json:"node_refresh_interval"`
	DataDir             string    `json:"data_dir"`
	OpenVPNCommand      string    `json:"openvpn_command"`
	TunnelBackend       string    `json:"tunnel_backend"`
	Channels            []Channel `json:"channels"`
}

type Channel struct {
	ID            string `json:"id"`
	ListenHost    string `json:"listen_host"`
	ListenPort    int    `json:"listen_port"`
	Region        string `json:"region"`
	RotateMinutes int    `json:"rotate_minutes"`
	SelectionMode string `json:"selection_mode"`
	ManualNodeID  string `json:"manual_node_id,omitempty"`
	Enabled       bool   `json:"enabled"`
}

func Default() Config {
	return Config{
		AdminHost:           "127.0.0.1",
		AdminPort:           8787,
		AdminPath:           "/admin",
		AdminUsername:       "admin",
		AdminPassword:       "change-me-admin",
		ProxyUsername:       "proxy",
		ProxyPassword:       "change-me-proxy",
		NodeRefreshInterval: "20m",
		DataDir:             "./data",
		OpenVPNCommand:      "openvpn",
		TunnelBackend:       TunnelBackendFake,
		Channels: []Channel{
			{
				ID:            "jp-3000",
				ListenHost:    "0.0.0.0",
				ListenPort:    3000,
				Region:        "jp",
				RotateMinutes: 10,
				SelectionMode: SelectionAuto,
				Enabled:       true,
			},
		},
	}
}

func LoadOrCreate(path string) (Config, error) {
	cfg, err := Load(path)
	if err == nil {
		return cfg, nil
	}
	if !os.IsNotExist(err) {
		return Config{}, err
	}
	cfg = Default()
	cfg.AdminPort = chooseAdminPort(cfg.AdminHost, cfg.AdminPort, usedChannelPorts(cfg.Channels))
	cfg.AdminPath = randomAdminPath()
	if err := Save(path, cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func randomAdminPath() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var builder strings.Builder
	builder.WriteString("/admin-")
	for i := 0; i < 16; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			builder.WriteByte('x')
			continue
		}
		builder.WriteByte(alphabet[n.Int64()])
	}
	return builder.String()
}

func chooseAdminPort(host string, fallback int, blocked map[int]struct{}) int {
	if isPortFree(host, fallback) {
		if _, ok := blocked[fallback]; !ok {
			return fallback
		}
	}
	for i := 0; i < 100; i++ {
		port := randomPort(20000, 60999)
		if _, ok := blocked[port]; ok {
			continue
		}
		if isPortFree(host, port) {
			return port
		}
	}
	return fallback
}

func usedChannelPorts(channels []Channel) map[int]struct{} {
	ports := make(map[int]struct{}, len(channels))
	for _, ch := range channels {
		ports[ch.ListenPort] = struct{}{}
	}
	return ports
}

func randomPort(min, max int) int {
	if max <= min {
		return min
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	if err != nil {
		return min
	}
	return min + int(n.Int64())
}

func isPortFree(host string, port int) bool {
	if host == "" {
		host = "127.0.0.1"
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	cfg.normalize()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	cfg.normalize()
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func (c *Config) normalize() {
	if c.AdminHost == "" {
		c.AdminHost = "127.0.0.1"
	}
	if c.AdminPath == "" {
		c.AdminPath = "/admin"
	}
	if !strings.HasPrefix(c.AdminPath, "/") {
		c.AdminPath = "/" + c.AdminPath
	}
	c.AdminPath = strings.TrimRight(c.AdminPath, "/")
	if c.AdminPath == "" {
		c.AdminPath = "/admin"
	}
	if c.DataDir == "" {
		c.DataDir = "./data"
	}
	if c.OpenVPNCommand == "" {
		c.OpenVPNCommand = "openvpn"
	}
	if c.TunnelBackend == "" {
		c.TunnelBackend = TunnelBackendFake
	}
	if c.NodeRefreshInterval == "" {
		c.NodeRefreshInterval = "20m"
	}
	for i := range c.Channels {
		c.Channels[i].ID = strings.TrimSpace(c.Channels[i].ID)
		c.Channels[i].Region = strings.ToLower(strings.TrimSpace(c.Channels[i].Region))
		if c.Channels[i].ListenHost == "" {
			c.Channels[i].ListenHost = "0.0.0.0"
		}
		if c.Channels[i].SelectionMode == "" {
			c.Channels[i].SelectionMode = SelectionAuto
		}
	}
}

func (c Config) Validate() error {
	if c.AdminPort < 1 || c.AdminPort > 65535 {
		return fmt.Errorf("admin port must be 1-65535")
	}
	if c.AdminPath == "/" {
		return fmt.Errorf("admin path must not be root")
	}
	if strings.Contains(c.AdminPath, " ") {
		return fmt.Errorf("admin path must not contain spaces")
	}
	if c.AdminUsername == "" {
		return fmt.Errorf("admin username is required")
	}
	if c.AdminPassword == "" {
		return fmt.Errorf("admin password is required")
	}
	if c.ProxyUsername == "" {
		return fmt.Errorf("proxy username is required")
	}
	if c.ProxyPassword == "" {
		return fmt.Errorf("proxy password is required")
	}
	switch c.TunnelBackend {
	case TunnelBackendFake, TunnelBackendOpenVPN:
	default:
		return fmt.Errorf("tunnel backend must be one of: fake, openvpn")
	}
	if len(c.Channels) == 0 {
		return fmt.Errorf("at least one channel is required")
	}

	ports := map[int]string{c.AdminPort: "admin"}
	ids := map[string]struct{}{}
	for _, ch := range c.Channels {
		if err := ch.Validate(); err != nil {
			return fmt.Errorf("channel %q: %w", ch.ID, err)
		}
		if _, ok := ids[ch.ID]; ok {
			return fmt.Errorf("duplicate channel id %q", ch.ID)
		}
		ids[ch.ID] = struct{}{}
		if owner, ok := ports[ch.ListenPort]; ok {
			return fmt.Errorf("port %d is used by both %s and channel %q", ch.ListenPort, owner, ch.ID)
		}
		ports[ch.ListenPort] = ch.ID
	}
	return nil
}

func (ch Channel) Validate() error {
	if ch.ID == "" {
		return fmt.Errorf("id is required")
	}
	if ch.ListenPort < 1 || ch.ListenPort > 65535 {
		return fmt.Errorf("listen port must be 1-65535")
	}
	if ch.Region == "" {
		return fmt.Errorf("region is required")
	}
	if ch.RotateMinutes < 0 {
		return fmt.Errorf("rotate minutes must be >= 0")
	}
	switch ch.SelectionMode {
	case SelectionAuto:
	case SelectionManual:
		if ch.ManualNodeID == "" {
			return fmt.Errorf("manual node id is required in manual mode")
		}
	default:
		return fmt.Errorf("selection mode must be auto or manual")
	}
	return nil
}
