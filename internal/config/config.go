package config

import "fmt"

type Config struct {
	ProxyHost            string   `json:"proxy_host"`
	ProxyPort            int      `json:"proxy_port"`
	AdminHost            string   `json:"admin_host"`
	AdminPort            int      `json:"admin_port"`
	AdminUsername        string   `json:"admin_username"`
	AdminPassword        string   `json:"admin_password"`
	ProxyPassword        string   `json:"proxy_password"`
	MaxActiveSessions    int      `json:"max_active_sessions"`
	IdleSessionTimeout   string   `json:"idle_session_timeout"`
	NodeRefreshInterval  string   `json:"node_refresh_interval"`
	AllowedRegions       []string `json:"allowed_regions"`
	AllowedRotateMinutes []int    `json:"allowed_rotate_minutes"`
	DataDir              string   `json:"data_dir"`
	OpenVPNCommand       string   `json:"openvpn_command"`
}

func Default() Config {
	return Config{
		ProxyHost:            "0.0.0.0",
		ProxyPort:            3000,
		AdminHost:            "0.0.0.0",
		AdminPort:            8787,
		AdminUsername:        "admin",
		AdminPassword:        "change-me-admin",
		ProxyPassword:        "change-me-proxy",
		MaxActiveSessions:    5,
		IdleSessionTimeout:   "10m",
		NodeRefreshInterval:  "20m",
		AllowedRegions:       []string{"jp", "us", "kr", "sg", "tw", "hk"},
		AllowedRotateMinutes: []int{0, 5, 10, 30, 60},
		DataDir:              "./data",
		OpenVPNCommand:       "openvpn",
	}
}

func (c Config) Validate() error {
	if c.ProxyPort < 1 || c.ProxyPort > 65535 {
		return fmt.Errorf("proxy port must be 1-65535")
	}
	if c.AdminPort < 1 || c.AdminPort > 65535 {
		return fmt.Errorf("admin port must be 1-65535")
	}
	if c.ProxyPort == c.AdminPort {
		return fmt.Errorf("proxy port and admin port must differ")
	}
	if c.AdminUsername == "" {
		return fmt.Errorf("admin username is required")
	}
	if c.AdminPassword == "" {
		return fmt.Errorf("admin password is required")
	}
	if c.ProxyPassword == "" {
		return fmt.Errorf("proxy password is required")
	}
	if c.MaxActiveSessions < 1 {
		return fmt.Errorf("max active sessions must be >= 1")
	}
	if len(c.AllowedRegions) == 0 {
		return fmt.Errorf("at least one region must be allowed")
	}
	if len(c.AllowedRotateMinutes) == 0 {
		return fmt.Errorf("at least one rotation value must be allowed")
	}
	return nil
}

func (c Config) AllowsRegion(region string) bool {
	for _, item := range c.AllowedRegions {
		if item == region {
			return true
		}
	}
	return false
}

func (c Config) AllowsRotateMinutes(minutes int) bool {
	for _, item := range c.AllowedRotateMinutes {
		if item == minutes {
			return true
		}
	}
	return false
}
