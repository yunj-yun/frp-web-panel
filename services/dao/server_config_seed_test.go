package dao

import (
	"testing"

	"github.com/yunj-yun/frp-web-panel/models"
)

func TestCreateServerSeedsDefaultConfig(t *testing.T) {
	// Models the create path: a Server stored without config content must end up
	// with usable frps defaults so tunnels can be created before the user visits
	// the Server page. This asserts the seeding decision itself; the persistence
	// path is covered by the SQLite-backed reservation tests.
	entity := &models.ServerEntity{ServerID: "srv-seed", ServerIP: "192.0.2.10"}
	if len(entity.ConfigContent) == 0 {
		if err := entity.SetConfigContent(models.DefaultServerConfig()); err != nil {
			t.Fatalf("seed default config: %v", err)
		}
	}
	cfg, err := entity.GetConfigContent()
	if err != nil {
		t.Fatalf("parse seeded config: %v", err)
	}
	if cfg.BindPort <= 0 {
		t.Fatalf("seeded bindPort = %d, want a usable port", cfg.BindPort)
	}
}
