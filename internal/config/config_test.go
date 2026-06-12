package config

import "testing"

func TestDefaultConfigIsValid(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Default config invalid: %v", err)
	}
	if cfg.AdminHost != "127.0.0.1" {
		t.Fatalf("AdminHost = %q, want 127.0.0.1", cfg.AdminHost)
	}
	if cfg.ProxyPort != 3000 {
		t.Fatalf("ProxyPort = %d, want 3000", cfg.ProxyPort)
	}
	if !cfg.AllowsRegion("jp") {
		t.Fatalf("default config should allow jp")
	}
	if !cfg.AllowsRotateMinutes(10) {
		t.Fatalf("default config should allow 10 minute rotation")
	}
}

func TestValidateRejectsBadPorts(t *testing.T) {
	cfg := Default()
	cfg.ProxyPort = 8787
	cfg.AdminPort = 8787
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected duplicate port error")
	}
}

func TestValidateRejectsEmptyPassword(t *testing.T) {
	cfg := Default()
	cfg.ProxyPassword = ""
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected empty proxy password error")
	}
}

func TestValidateAcceptsKnownTunnelBackends(t *testing.T) {
	for _, backend := range []string{"fake", "openvpn"} {
		t.Run(backend, func(t *testing.T) {
			cfg := Default()
			cfg.TunnelBackend = backend
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate returned error for %q: %v", backend, err)
			}
		})
	}
}

func TestValidateRejectsUnknownTunnelBackend(t *testing.T) {
	cfg := Default()
	cfg.TunnelBackend = "wireguard"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected unknown tunnel backend error")
	}
}
