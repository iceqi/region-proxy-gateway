package config

import (
	"net"
	"path/filepath"
	"testing"
)

func TestDefaultConfigIsValid(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Default config invalid: %v", err)
	}
	if cfg.AdminHost != "127.0.0.1" {
		t.Fatalf("AdminHost = %q, want 127.0.0.1", cfg.AdminHost)
	}
	if len(cfg.Channels) != 1 {
		t.Fatalf("default channels = %d, want 1", len(cfg.Channels))
	}
	if cfg.Channels[0].ListenPort != 3000 {
		t.Fatalf("default channel port = %d, want 3000", cfg.Channels[0].ListenPort)
	}
	if cfg.Channels[0].Region != "jp" {
		t.Fatalf("default channel region = %q, want jp", cfg.Channels[0].Region)
	}
}

func TestValidateRejectsDuplicateChannelPorts(t *testing.T) {
	cfg := Default()
	cfg.Channels = append(cfg.Channels, Channel{
		ID:            "us",
		ListenHost:    "0.0.0.0",
		ListenPort:    cfg.Channels[0].ListenPort,
		Region:        "us",
		SelectionMode: SelectionAuto,
		Enabled:       true,
	})

	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected duplicate channel port error")
	}
}

func TestValidateRejectsAdminPortCollision(t *testing.T) {
	cfg := Default()
	cfg.AdminPort = cfg.Channels[0].ListenPort
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected admin/channel port collision error")
	}
}

func TestValidateRejectsInvalidChannel(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Channel)
	}{
		{name: "empty id", mutate: func(ch *Channel) { ch.ID = "" }},
		{name: "bad port", mutate: func(ch *Channel) { ch.ListenPort = 70000 }},
		{name: "empty region", mutate: func(ch *Channel) { ch.Region = "" }},
		{name: "negative rotate", mutate: func(ch *Channel) { ch.RotateMinutes = -1 }},
		{name: "bad selection mode", mutate: func(ch *Channel) { ch.SelectionMode = "fastest-ish" }},
		{name: "manual missing node", mutate: func(ch *Channel) {
			ch.SelectionMode = SelectionManual
			ch.ManualNodeID = ""
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.mutate(&cfg.Channels[0])
			if err := cfg.Validate(); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestLoadOrCreateWritesDefaultAndRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	cfg, err := LoadOrCreate(path)
	if err != nil {
		t.Fatalf("LoadOrCreate default: %v", err)
	}
	if len(cfg.Channels) != 1 {
		t.Fatalf("channels = %d, want 1", len(cfg.Channels))
	}

	cfg.Channels[0].Region = "us"
	cfg.Channels[0].RotateMinutes = 10
	cfg.Channels[0].SelectionMode = SelectionAuto
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Channels[0].Region != "us" {
		t.Fatalf("region = %q, want us", loaded.Channels[0].Region)
	}
	if loaded.Channels[0].RotateMinutes != 10 {
		t.Fatalf("rotate minutes = %d, want 10", loaded.Channels[0].RotateMinutes)
	}
}

func TestLoadOrCreateChoosesRandomFreeAdminPortWhenDefaultIsBusy(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:8787")
	if err != nil {
		t.Skipf("default admin port already busy: %v", err)
	}
	defer listener.Close()

	cfg, err := LoadOrCreate(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatalf("LoadOrCreate: %v", err)
	}
	if cfg.AdminPort == 8787 {
		t.Fatalf("admin port = 8787, want a random free port")
	}
	if cfg.AdminPort < 20000 || cfg.AdminPort > 60999 {
		t.Fatalf("admin port = %d, want random high port", cfg.AdminPort)
	}
}
