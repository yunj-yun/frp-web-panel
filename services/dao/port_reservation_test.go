package dao

import (
	"testing"

	"github.com/yunj-yun/frp-web-panel/models"
	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestReleaseSharedProxyReservationAfterLastReference(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.PortReservation{}, &models.ProxyConfig{}); err != nil {
		t.Fatal(err)
	}

	userInfo := &models.UserEntity{UserID: 7, TenantID: 9}
	activeKey := models.PortReservationActiveKey("server-1", models.NodeTypeServer, "tcp", 18891)
	reservation := &models.PortReservation{
		NodeID: "server-1", NodeType: models.NodeTypeServer, Protocol: "tcp", Port: 18891,
		ActiveKey: &activeKey, ResourceType: models.PortReservationTypeProxy,
		ResourceID: "server-1/origin-1/shared", UserID: userInfo.UserID, TenantID: userInfo.TenantID,
		Status: models.PortReservationStatusActive,
	}
	if err := db.Create(reservation).Error; err != nil {
		t.Fatal(err)
	}

	for _, clientID := range []string{"shadow-1", "shadow-2"} {
		content, err := (&v1.TypedProxyConfig{
			Type: string(v1.ProxyTypeTCP),
			ProxyConfigurer: &v1.TCPProxyConfig{
				ProxyBaseConfig: v1.ProxyBaseConfig{Name: "shared", Type: string(v1.ProxyTypeTCP)},
				RemotePort:      18891,
			},
		}).MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&models.ProxyConfig{ProxyConfigEntity: &models.ProxyConfigEntity{
			ServerID: "server-1", ClientID: clientID, OriginClientID: "origin-1", Name: "shared",
			UserID: userInfo.UserID, TenantID: userInfo.TenantID, Content: content,
		}}).Error; err != nil {
			t.Fatal(err)
		}
	}

	removeReference := func(clientID string) error {
		return db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("client_id = ?", clientID).Unscoped().Delete(&models.ProxyConfig{}).Error; err != nil {
				return err
			}
			return releaseProxyReservationRefsIfUnusedTx(tx, userInfo, []proxyPortReservationRef{{
				nodeID: "server-1", nodeType: models.NodeTypeServer, protocol: "tcp", port: 18891,
				resourceID: "server-1/origin-1/shared",
			}})
		})
	}

	if err := removeReference("shadow-1"); err != nil {
		t.Fatal(err)
	}
	var current models.PortReservation
	if err := db.First(&current, reservation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.Status != models.PortReservationStatusActive {
		t.Fatalf("reservation released while a shadow reference remained: %s", current.Status)
	}

	if err := removeReference("shadow-2"); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&current, reservation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.Status != models.PortReservationStatusReleased {
		t.Fatalf("reservation status = %s, want released", current.Status)
	}
}
