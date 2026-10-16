package models

import (
	"errors"
	"fmt"
	"strings"
	"time"

	v1 "github.com/fatedier/frp/pkg/config/v1"
	"gorm.io/gorm"
)

const (
	PortReservationStatusActive   = "active"
	PortReservationStatusReleased = "released"
	PortReservationTypeProxy      = "proxy"
	PortReservationTypeFRPS       = "frps"
	PortReservationTypeCaddy      = "caddy"
	PortReservationTypeGateway    = "gateway"
	PortReservationTypeLegacy     = "legacy"
)

const (
	CaddyHTTPPort  uint16 = 80
	CaddyHTTPSPort uint16 = 443
	NodeTypeClient        = "client"
	NodeTypeServer        = "server"
)

// NodePortSnapshot stores the latest point-in-time socket observations sent by
// a Linux Server or Client Agent.
type NodePortSnapshot struct {
	ID         uint   `gorm:"primaryKey"`
	ScanID     uint   `gorm:"index;not null"`
	NodeID     string `gorm:"index:idx_node_port_snapshot,priority:1;not null"`
	NodeType   string `gorm:"index:idx_node_port_snapshot,priority:2;not null"`
	UserID     int    `gorm:"index"`
	TenantID   int    `gorm:"index"`
	Protocol   string `gorm:"index:idx_node_port_snapshot,priority:3;not null"`
	Address    string `gorm:"not null"`
	Port       uint16 `gorm:"index:idx_node_port_snapshot,priority:4;not null"`
	Process    string
	PID        int
	ObservedAt time.Time `gorm:"index"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (NodePortSnapshot) TableName() string { return "node_port_snapshots" }

type NodePortScan struct {
	ID          uint      `gorm:"primaryKey"`
	NodeID      string    `gorm:"uniqueIndex:idx_node_port_scan,priority:1;not null"`
	NodeType    string    `gorm:"uniqueIndex:idx_node_port_scan,priority:2;not null"`
	UserID      int       `gorm:"index"`
	TenantID    int       `gorm:"index"`
	ObservedAt  time.Time `gorm:"index"`
	SocketCount int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (NodePortScan) TableName() string { return "node_port_scans" }

// PortReservation represents a port assigned or being prepared by the
// platform. It is deliberately separate from a socket snapshot: a reservation
// is a platform intent, while a snapshot is an operating-system observation.
type PortReservation struct {
	ID           uint    `gorm:"primaryKey"`
	NodeID       string  `gorm:"index:idx_port_reservation_node,priority:1;not null"`
	NodeType     string  `gorm:"index:idx_port_reservation_node,priority:2;not null"`
	Protocol     string  `gorm:"index:idx_port_reservation_node,priority:3;not null"`
	Port         uint16  `gorm:"index:idx_port_reservation_node,priority:4;not null"`
	ActiveKey    *string `gorm:"uniqueIndex:idx_port_reservation_active_key"`
	ResourceType string  `gorm:"not null"`
	ResourceID   string  `gorm:"not null"`
	UserID       int     `gorm:"index"`
	TenantID     int     `gorm:"index"`
	Status       string  `gorm:"index;not null"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ReleasedAt   *time.Time
}

func (PortReservation) TableName() string { return "port_reservations" }

func PortReservationActiveKey(nodeID, nodeType, protocol string, port uint16) string {
	return fmt.Sprintf("%d:%s|%d:%s|%s|%d", len(nodeType), nodeType, len(nodeID), nodeID, protocol, port)
}

// BackfillActivePortReservationKeys upgrades older reservation rows while
// preserving duplicate or released rows as history. When old tenant-scoped
// data contains duplicate physical listeners, the earliest active row remains
// authoritative and later duplicates are marked released for manual review.
func BackfillActivePortReservationKeys(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&PortReservation{}).Where("status = ?", PortReservationStatusActive).UpdateColumn("active_key", nil).Error; err != nil {
			return err
		}
		if err := tx.Model(&PortReservation{}).Where("status <> ?", PortReservationStatusActive).UpdateColumn("active_key", nil).Error; err != nil {
			return err
		}
		var reservations []*PortReservation
		if err := tx.Where("status = ?", PortReservationStatusActive).Order("id ASC").Find(&reservations).Error; err != nil {
			return err
		}
		seen := make(map[string]struct{}, len(reservations))
		now := time.Now()
		for _, reservation := range reservations {
			if reservation == nil || !isSupportedReservationProtocol(reservation.Protocol) || reservation.NodeID == "" || reservation.NodeType == "" || reservation.Port == 0 {
				return fmt.Errorf("invalid active port reservation id %d", reservation.ID)
			}
			key := PortReservationActiveKey(reservation.NodeID, reservation.NodeType, reservation.Protocol, reservation.Port)
			if _, duplicate := seen[key]; duplicate {
				if err := tx.Model(&PortReservation{}).Where("id = ?", reservation.ID).Updates(map[string]interface{}{
					"status": PortReservationStatusReleased, "released_at": now, "updated_at": now,
				}).Error; err != nil {
					return err
				}
				continue
			}
			seen[key] = struct{}{}
			if err := tx.Model(&PortReservation{}).Where("id = ?", reservation.ID).UpdateColumn("active_key", key).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func isSupportedReservationProtocol(protocol string) bool {
	return protocol == "tcp" || protocol == "udp"
}

func addPortReservation(db *gorm.DB, serverID string, userID, tenantID int, protocol string, port uint16, resourceType, resourceID string) error {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if strings.TrimSpace(serverID) == "" || port == 0 || !isSupportedReservationProtocol(protocol) || resourceType == "" || resourceID == "" {
		return fmt.Errorf("invalid port reservation for %s:%d", protocol, port)
	}
	var existing PortReservation
	err := db.Where(&PortReservation{
		NodeID: serverID, NodeType: NodeTypeServer, Protocol: protocol, Port: port, Status: PortReservationStatusActive,
	}).First(&existing).Error
	if err == nil {
		if existing.ResourceType == resourceType && existing.ResourceID == resourceID {
			return nil
		}
		return fmt.Errorf("%s port %d is already reserved by %s %s", protocol, port, existing.ResourceType, existing.ResourceID)
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}
	activeKey := PortReservationActiveKey(serverID, NodeTypeServer, protocol, port)
	return db.Create(&PortReservation{
		NodeID: serverID, NodeType: NodeTypeServer, Protocol: protocol, Port: port,
		ActiveKey:    &activeKey,
		ResourceType: resourceType, ResourceID: resourceID,
		UserID: userID, TenantID: tenantID, Status: PortReservationStatusActive,
	}).Error
}

// PortReservationSpec describes a listener that belongs to an FRPS process.
// Protocol is part of the identity: TCP and UDP may use the same numeric port
// on a host, while two listeners using the same protocol cannot share it.
type PortReservationSpec struct {
	Protocol string
	Port     uint16
	Name     string
}

func ServerPortReservations(config *v1.ServerConfig) []PortReservationSpec {
	if config == nil {
		return nil
	}
	ports := make([]PortReservationSpec, 0, 8)
	add := func(protocol string, port int, name string) {
		if port > 0 && port <= 65535 {
			ports = append(ports, PortReservationSpec{Protocol: protocol, Port: uint16(port), Name: name})
		}
	}
	add("tcp", config.BindPort, "bindPort")
	add("udp", config.KCPBindPort, "kcpBindPort")
	add("udp", config.QUICBindPort, "quicBindPort")
	add("tcp", config.VhostHTTPPort, "vhostHTTPPort")
	add("tcp", config.VhostHTTPSPort, "vhostHTTPSPort")
	add("tcp", config.TCPMuxHTTPConnectPort, "tcpmuxHTTPConnectPort")
	add("tcp", config.WebServer.Port, "webServer.port")
	add("tcp", config.SSHTunnelGateway.BindPort, "sshTunnelGateway.bindPort")
	return ports
}

func CaddyPortReservations() []PortReservationSpec {
	return []PortReservationSpec{
		{Protocol: "tcp", Port: CaddyHTTPPort, Name: "caddy.http"},
		{Protocol: "tcp", Port: CaddyHTTPSPort, Name: "caddy.https"},
	}
}

// ServerPortReservationConflict returns the owner label for a fixed listener
// that conflicts with protocol/port. It intentionally includes Caddy's public
// listeners because the Server agent runs FRPS and Caddy on the same host.
func ServerPortReservationConflict(config *v1.ServerConfig, protocol string, port uint16) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "" || port == 0 {
		return ""
	}
	for _, item := range CaddyPortReservations() {
		if item.Protocol == protocol && item.Port == port {
			return item.Name
		}
	}
	return FRPServerPortReservationConflict(config, protocol, port)
}

// FRPServerPortReservationConflict only checks listeners owned by frps. Caddy
// gateway routes share Caddy's 80/443 sockets, so those listeners must not be
// treated as conflicts when validating a route's public listen port.
func FRPServerPortReservationConflict(config *v1.ServerConfig, protocol string, port uint16) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "" || port == 0 {
		return ""
	}
	for _, item := range ServerPortReservations(config) {
		if item.Protocol == protocol && item.Port == port {
			return "frps " + item.Name
		}
	}
	return ""
}

func proxyRemotePortReservation(proxy *ProxyConfig) (string, uint16, bool) {
	if proxy == nil || proxy.ProxyConfigEntity == nil || proxy.Stopped {
		return "", 0, false
	}
	cfg, err := proxy.GetTypedProxyConfig()
	if err != nil || cfg.ProxyConfigurer == nil || cfg.GetBaseConfig() == nil {
		return "", 0, false
	}
	protocol := cfg.GetBaseConfig().Type
	var port int
	switch typed := cfg.ProxyConfigurer.(type) {
	case *v1.TCPProxyConfig:
		protocol, port = "tcp", typed.RemotePort
	case *v1.UDPProxyConfig:
		protocol, port = "udp", typed.RemotePort
	default:
		return "", 0, false
	}
	if port <= 0 || port > 65535 {
		return protocol, 0, false
	}
	return protocol, uint16(port), true
}

func proxyReservationID(proxy *ProxyConfig) string {
	owner := strings.TrimSpace(proxy.OriginClientID)
	if owner == "" {
		owner = strings.TrimSpace(proxy.ClientID)
	}
	return fmt.Sprintf("%s/%s/%s", strings.TrimSpace(proxy.ServerID), owner, strings.TrimSpace(proxy.Name))
}

// BackfillPortReservations creates missing physical-node reservations for
// existing server listeners, active TCP/UDP tunnels and Caddy gateway ports.
// It is idempotent and intentionally leaves historical rows untouched. A
// legacy configuration can contain conflicts, so one conflict is reported but
// does not prevent independent records from being backfilled.
func BackfillPortReservations(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}
	var backfillErrors []error
	recordError := func(scope string, err error) {
		if err != nil {
			backfillErrors = append(backfillErrors, fmt.Errorf("%s: %w", scope, err))
		}
	}

	var servers []*Server
	if err := db.Find(&servers).Error; err != nil {
		return err
	}
	validServers := make(map[string]*Server, len(servers))
	serverBackfillComplete := make(map[string]bool, len(servers))
	expected := make(map[string]struct{})
	type backfillSpec struct {
		nodeID, nodeType, protocol string
		port                       uint16
		resourceType, resourceID   string
		userID, tenantID           int
	}
	specs := make([]backfillSpec, 0)
	markExpected := func(spec backfillSpec) {
		expected[portReservationIdentity(spec.nodeID, spec.nodeType, spec.protocol, spec.port, spec.resourceType, spec.resourceID)] = struct{}{}
		specs = append(specs, spec)
	}
	routesToBackfill := make([]*GatewayRoute, 0)
	for _, server := range servers {
		if server == nil || server.ServerEntity == nil || strings.TrimSpace(server.ServerID) == "" {
			continue
		}
		validServers[server.ServerID] = server
		for _, item := range CaddyPortReservations() {
			resourceID := server.ServerID + "/" + strings.TrimPrefix(item.Name, "caddy.")
			markExpected(backfillSpec{server.ServerID, NodeTypeServer, item.Protocol, item.Port, PortReservationTypeCaddy, resourceID, server.UserID, server.TenantID})
		}
		config, err := server.GetConfigContent()
		if err != nil && len(server.ConfigContent) > 0 {
			recordError("read server "+server.ServerID+" configuration", err)
			continue
		}
		serverBackfillComplete[server.ServerID] = true
		for _, item := range ServerPortReservations(config) {
			resourceID := server.ServerID + "/" + item.Name
			markExpected(backfillSpec{server.ServerID, NodeTypeServer, item.Protocol, item.Port, PortReservationTypeFRPS, resourceID, server.UserID, server.TenantID})
		}
	}

	var proxies []*ProxyConfig
	if err := db.Where("stopped IS NULL OR stopped = ?", false).Find(&proxies).Error; err != nil {
		return err
	}
	for _, proxy := range proxies {
		if proxy == nil || proxy.ProxyConfigEntity == nil || proxy.ServerID == "" || validServers[proxy.ServerID] == nil {
			continue
		}
		protocol, port, ok := proxyRemotePortReservation(proxy)
		if !ok {
			continue
		}
		resourceID := proxyReservationID(proxy)
		markExpected(backfillSpec{proxy.ServerID, NodeTypeServer, protocol, port, PortReservationTypeProxy, resourceID, proxy.UserID, proxy.TenantID})
	}

	var routes []*GatewayRoute
	if err := db.Where("deleted_at IS NULL").Find(&routes).Error; err != nil {
		return err
	}
	for _, route := range routes {
		if route == nil || route.ServerID == "" || validServers[route.ServerID] == nil || route.ListenPort == CaddyHTTPPort || route.ListenPort == CaddyHTTPSPort {
			continue
		}
		resourceID := GatewayRoutePortResourceID(route.ServerID, route.ListenPort)
		markExpected(backfillSpec{route.ServerID, NodeTypeServer, "tcp", route.ListenPort, PortReservationTypeGateway, resourceID, route.UserID, route.TenantID})
		routesToBackfill = append(routesToBackfill, route)
	}

	// Remove active reservations that point to deleted servers or to proxy/
	// gateway records no longer present in the database. Historical released
	// rows remain untouched for audit purposes.
	var activeReservations []*PortReservation
	if err := db.Where("status = ? AND node_type = ?", PortReservationStatusActive, NodeTypeServer).Find(&activeReservations).Error; err != nil {
		return err
	}
	staleIDs := make([]uint, 0)
	for _, reservation := range activeReservations {
		if reservation == nil {
			continue
		}
		if validServers[reservation.NodeID] == nil {
			staleIDs = append(staleIDs, reservation.ID)
			continue
		}
		if !serverBackfillComplete[reservation.NodeID] {
			continue
		}
		if reservation.ResourceType != PortReservationTypeProxy && reservation.ResourceType != PortReservationTypeGateway &&
			reservation.ResourceType != PortReservationTypeFRPS && reservation.ResourceType != PortReservationTypeCaddy {
			staleIDs = append(staleIDs, reservation.ID)
			continue
		}
		if _, ok := expected[portReservationIdentity(reservation.NodeID, reservation.NodeType, reservation.Protocol, reservation.Port,
			reservation.ResourceType, reservation.ResourceID)]; !ok {
			staleIDs = append(staleIDs, reservation.ID)
		}
	}
	if len(staleIDs) > 0 {
		now := time.Now()
		recordError("release stale port reservations", db.Model(&PortReservation{}).Where("id IN ?", staleIDs).Updates(map[string]interface{}{
			"status":      PortReservationStatusReleased,
			"active_key":  nil,
			"released_at": now,
			"updated_at":  now,
		}).Error)
	}
	for _, spec := range specs {
		if spec.resourceType == PortReservationTypeGateway {
			continue
		}
		recordError(fmt.Sprintf("backfill %s %s", spec.resourceType, spec.resourceID),
			addPortReservation(db, spec.nodeID, spec.userID, spec.tenantID, spec.protocol, spec.port, spec.resourceType, spec.resourceID))
	}
	for _, route := range routesToBackfill {
		recordError(fmt.Sprintf("backfill gateway route %d", route.ID), backfillGatewayRoutePortReservation(db, route))
	}
	return errors.Join(backfillErrors...)
}

func portReservationIdentity(nodeID, nodeType, protocol string, port uint16, resourceType, resourceID string) string {
	// Reservations are physically unique per node/protocol/port. Tenant
	// metadata describes the owner but must not change the reservation key.
	return fmt.Sprintf("%s|%s|%s|%d|%s|%s", nodeID, nodeType, protocol, port, resourceType, resourceID)
}

// GatewayRoutePortResourceID gives every route sharing a Server listener the
// same reservation identity. A Server TCP port can be served by multiple
// Caddy host routes, but still represents one physical socket.
func GatewayRoutePortResourceID(serverID string, port uint16) string {
	return fmt.Sprintf("%s/listen/%d", strings.TrimSpace(serverID), port)
}

func backfillGatewayRoutePortReservation(db *gorm.DB, route *GatewayRoute) error {
	if db == nil || route == nil || strings.TrimSpace(route.ServerID) == "" || route.ListenPort == 0 {
		return fmt.Errorf("invalid gateway route port reservation")
	}
	resourceID := GatewayRoutePortResourceID(route.ServerID, route.ListenPort)
	var existing PortReservation
	err := db.Where(&PortReservation{
		NodeID: route.ServerID, NodeType: NodeTypeServer, Protocol: "tcp", Port: route.ListenPort,
		Status: PortReservationStatusActive,
	}).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return addPortReservation(db, route.ServerID, route.UserID, route.TenantID, "tcp", route.ListenPort, PortReservationTypeGateway, resourceID)
	}
	if err != nil {
		return err
	}
	if existing.ResourceType != PortReservationTypeGateway {
		return fmt.Errorf("tcp port %d is already reserved by %s %s", route.ListenPort, existing.ResourceType, existing.ResourceID)
	}
	if existing.ResourceID == resourceID {
		return nil
	}
	// Older builds stored one reservation per route as route/<id>. Convert
	// that active row in place so existing databases keep their reservation and
	// history while new writes can share it across domains.
	if !strings.HasPrefix(existing.ResourceID, "route/") {
		return fmt.Errorf("tcp port %d has an unrecognized gateway reservation %s", route.ListenPort, existing.ResourceID)
	}
	return db.Model(&PortReservation{}).Where("id = ?", existing.ID).Updates(map[string]interface{}{
		"resource_id": resourceID,
		"user_id":     route.UserID,
		"tenant_id":   route.TenantID,
		"updated_at":  time.Now(),
	}).Error
}
