package models

import "time"

const (
	GatewayRouteStatusPending  = "pending"
	GatewayRouteStatusActive   = "active"
	GatewayRouteStatusError    = "error"
	GatewayRouteStatusDisabled = "disabled"
)

// GatewayRoute maps a public hostname on a Server's Caddy instance to an
// HTTP FRP proxy. Caddy terminates TLS and forwards the original Host header
// to frps' vhostHTTPPort, so the client-side local service remains unchanged.
type GatewayRoute struct {
	ID            uint   `gorm:"primaryKey"`
	UserID        int    `gorm:"index;not null"`
	TenantID      int    `gorm:"index;not null"`
	Domain        string `gorm:"type:varchar(255);index;not null"`
	ListenPort    uint16 `gorm:"not null;default:443"`
	ServerID      string `gorm:"type:varchar(255);index;not null"`
	ProxyID       uint   `gorm:"index;not null"`
	CertificateID uint   `gorm:"index;not null"`
	UpstreamPort  uint16 `gorm:"not null"`
	Status        string `gorm:"type:varchar(32);index;not null"`
	ErrorMessage  string `gorm:"type:text"`
	LastAppliedAt *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     *time.Time `gorm:"index"`
}

func (GatewayRoute) TableName() string { return "gateway_routes" }
