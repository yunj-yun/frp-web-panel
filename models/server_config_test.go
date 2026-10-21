package models

import "testing"

func TestDefaultServerConfigIsUsableByClients(t *testing.T) {
	cfg := DefaultServerConfig()
	if cfg == nil {
		t.Fatal("DefaultServerConfig() returned nil")
	}
	// The frp client connects to bindPort, so a Server seeded with these
	// defaults must advertise a real port rather than zero.
	if cfg.BindPort <= 0 || cfg.BindPort > 65535 {
		t.Fatalf("default bindPort = %d, want a usable port", cfg.BindPort)
	}
	if cfg.BindAddr == "" {
		t.Fatal("default bindAddr is empty")
	}
}

func TestDefaultServerConfigIsIndependentPerCall(t *testing.T) {
	first := DefaultServerConfig()
	first.BindPort = 12345
	second := DefaultServerConfig()
	// Callers mutate the returned config before saving; a shared pointer would
	// leak one Server's edits into the next.
	if second.BindPort == 12345 {
		t.Fatal("DefaultServerConfig() returns a shared instance")
	}
}

func TestSeededConfigRoundTripsThroughServerEntity(t *testing.T) {
	entity := &ServerEntity{}
	if err := entity.SetConfigContent(DefaultServerConfig()); err != nil {
		t.Fatalf("set config content: %v", err)
	}
	if len(entity.ConfigContent) == 0 {
		t.Fatal("seeded config content is empty")
	}
	parsed, err := entity.GetConfigContent()
	if err != nil {
		t.Fatalf("parse seeded config: %v", err)
	}
	if parsed.BindPort != DefaultServerConfig().BindPort {
		t.Fatalf("bindPort changed across round trip: %d", parsed.BindPort)
	}
}
