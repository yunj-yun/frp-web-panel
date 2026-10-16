package dao

import (
	"fmt"
	"strings"
	"time"

	"github.com/yunj-yun/frp-web-panel/models"
	v1 "github.com/fatedier/frp/pkg/config/v1"
	"gorm.io/gorm"
)

type PortSnapshotQuery interface {
	GetNodePortScans(userInfo models.UserInfo, clientIDs, serverIDs []string) ([]*models.NodePortScan, error)
	GetNodePortSockets(scanIDs []uint) ([]*models.NodePortSnapshot, error)
}

type PortSnapshotMutation interface {
	AdminReplaceNodePortSnapshot(nodeID, nodeType string, userID, tenantID int, observedAt time.Time, sockets []*models.NodePortSnapshot) error
}

// PortReservationQuery and PortReservationMutation are kept separate from
// socket snapshots because an observed listener and a platform reservation
// represent different facts. Reservations are used by configuration writes
// to serialize port ownership across concurrent requests.
type PortReservationQuery interface {
	ListActivePortReservations(userInfo models.UserInfo, nodeID, nodeType string) ([]*models.PortReservation, error)
}

type PortReservationMutation interface {
	ReservePort(userInfo models.UserInfo, nodeID, nodeType, protocol string, port uint16, resourceType, resourceID string, aliases ...string) (bool, error)
	ReleasePort(userInfo models.UserInfo, nodeID, nodeType, protocol string, port uint16, resourceType, resourceID string) error
	ReleaseProxyPortIfUnused(userInfo models.UserInfo, nodeID, nodeType, protocol string, port uint16, resourceID string) error
	ReleaseResourcePorts(userInfo models.UserInfo, resourceType, resourceID string) error
	ReleaseNodePorts(userInfo models.UserInfo, nodeID, nodeType string) error
	ReconcileServerPortReservations(userInfo models.UserInfo, serverID string, config *v1.ServerConfig) error
}

type portSnapshotQuery struct{ *queryImpl }
type portSnapshotMutation struct{ *mutationImpl }
type portReservationQuery struct{ *queryImpl }
type portReservationMutation struct{ *mutationImpl }

func newPortSnapshotQuery(base *queryImpl) PortSnapshotQuery {
	return &portSnapshotQuery{base}
}

func newPortSnapshotMutation(base *mutationImpl) PortSnapshotMutation {
	return &portSnapshotMutation{base}
}

func newPortReservationQuery(base *queryImpl) PortReservationQuery {
	return &portReservationQuery{base}
}

func newPortReservationMutation(base *mutationImpl) PortReservationMutation {
	return &portReservationMutation{base}
}

func (q *portSnapshotQuery) GetNodePortScans(userInfo models.UserInfo, clientIDs, serverIDs []string) ([]*models.NodePortScan, error) {
	if userInfo == nil {
		return nil, fmt.Errorf("user info is required")
	}
	if len(clientIDs) == 0 && len(serverIDs) == 0 {
		return []*models.NodePortScan{}, nil
	}

	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	query := db.Where("user_id = ? AND tenant_id = ?", userInfo.GetUserID(), userInfo.GetTenantID())
	switch {
	case len(clientIDs) > 0 && len(serverIDs) > 0:
		query = query.Where("(node_type = ? AND node_id IN ?) OR (node_type = ? AND node_id IN ?)", "client", clientIDs, "server", serverIDs)
	case len(clientIDs) > 0:
		query = query.Where("node_type = ? AND node_id IN ?", "client", clientIDs)
	default:
		query = query.Where("node_type = ? AND node_id IN ?", "server", serverIDs)
	}

	var scans []*models.NodePortScan
	if err := query.Order("observed_at DESC").Find(&scans).Error; err != nil {
		return nil, err
	}
	return scans, nil
}

func (q *portSnapshotQuery) GetNodePortSockets(scanIDs []uint) ([]*models.NodePortSnapshot, error) {
	if len(scanIDs) == 0 {
		return []*models.NodePortSnapshot{}, nil
	}
	var sockets []*models.NodePortSnapshot
	err := q.ctx.GetApp().GetDBManager().GetDefaultDB().Where("scan_id IN ?", scanIDs).Order("protocol, port, address").Find(&sockets).Error
	return sockets, err
}

func (m *portSnapshotMutation) AdminReplaceNodePortSnapshot(nodeID, nodeType string, userID, tenantID int, observedAt time.Time, sockets []*models.NodePortSnapshot) error {
	if nodeID == "" || nodeType == "" {
		return fmt.Errorf("invalid node port snapshot identity")
	}
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	validSockets := make([]*models.NodePortSnapshot, 0, len(sockets))
	for _, socket := range sockets {
		if socket != nil {
			validSockets = append(validSockets, socket)
		}
	}

	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	return db.Transaction(func(tx *gorm.DB) error {
		var scan models.NodePortScan
		err := tx.Where(&models.NodePortScan{NodeID: nodeID, NodeType: nodeType}).First(&scan).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		if err == gorm.ErrRecordNotFound {
			scan = models.NodePortScan{NodeID: nodeID, NodeType: nodeType}
		} else if scan.ObservedAt.After(observedAt) {
			return nil
		}
		scan.UserID = userID
		scan.TenantID = tenantID
		scan.ObservedAt = observedAt
		scan.SocketCount = len(validSockets)
		if err := tx.Save(&scan).Error; err != nil {
			return err
		}
		if err := tx.Where("scan_id = ?", scan.ID).Delete(&models.NodePortSnapshot{}).Error; err != nil {
			return err
		}
		for _, socket := range validSockets {
			socket.ID = 0
			socket.ScanID = scan.ID
			socket.NodeID = nodeID
			socket.NodeType = nodeType
			socket.UserID = userID
			socket.TenantID = tenantID
			socket.ObservedAt = observedAt
		}
		if len(validSockets) == 0 {
			return nil
		}
		return tx.CreateInBatches(validSockets, 100).Error
	})
}

func (q *portReservationQuery) ListActivePortReservations(userInfo models.UserInfo, nodeID, nodeType string) ([]*models.PortReservation, error) {
	if userInfo == nil || !userInfo.Valid() {
		return nil, fmt.Errorf("user info is required")
	}
	nodeID = strings.TrimSpace(nodeID)
	nodeType = strings.TrimSpace(nodeType)
	if nodeID == "" || nodeType == "" {
		return nil, fmt.Errorf("node id and node type are required")
	}
	var reservations []*models.PortReservation
	err := q.ctx.GetApp().GetDBManager().GetDefaultDB().Where(&models.PortReservation{
		NodeID:   nodeID,
		NodeType: nodeType,
		UserID:   userInfo.GetUserID(),
		TenantID: userInfo.GetTenantID(),
		Status:   models.PortReservationStatusActive,
	}).Order("protocol, port, resource_type, resource_id").Find(&reservations).Error
	return reservations, err
}

func (m *portReservationMutation) ReservePort(userInfo models.UserInfo, nodeID, nodeType, protocol string, port uint16, resourceType, resourceID string, aliases ...string) (bool, error) {
	if userInfo == nil || !userInfo.Valid() {
		return false, fmt.Errorf("user info is required")
	}
	nodeID = strings.TrimSpace(nodeID)
	nodeType = strings.TrimSpace(nodeType)
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	resourceType = strings.TrimSpace(resourceType)
	resourceID = strings.TrimSpace(resourceID)
	if nodeID == "" || nodeType == "" || (protocol != "tcp" && protocol != "udp") || port == 0 || resourceType == "" || resourceID == "" {
		return false, fmt.Errorf("invalid port reservation")
	}

	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	created := false
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		created, err = reservePortTx(tx, userInfo, nodeID, nodeType, protocol, port, resourceType, resourceID, aliases...)
		return err
	})
	return created, err
}

// reservePortTx is the transaction-safe implementation shared by ordinary
// port writes and full client-config rebuilds. The caller owns the transaction.
func reservePortTx(tx *gorm.DB, userInfo models.UserInfo, nodeID, nodeType, protocol string, port uint16, resourceType, resourceID string, aliases ...string) (bool, error) {
	var existing models.PortReservation
	err := tx.Where(&models.PortReservation{
		NodeID: nodeID, NodeType: nodeType, Protocol: protocol, Port: port,
		Status: models.PortReservationStatusActive,
	}).First(&existing).Error
	if err == nil {
		if existing.ResourceType != resourceType || existing.ResourceID != resourceID {
			for _, alias := range aliases {
				if existing.ResourceType != resourceType || strings.TrimSpace(alias) == "" || existing.ResourceID != strings.TrimSpace(alias) {
					continue
				}
				existing.ResourceID = resourceID
				existing.UserID = userInfo.GetUserID()
				existing.TenantID = userInfo.GetTenantID()
				existing.UpdatedAt = time.Now()
				existing.ReleasedAt = nil
				activeKey := models.PortReservationActiveKey(nodeID, nodeType, protocol, port)
				existing.ActiveKey = &activeKey
				return false, tx.Save(&existing).Error
			}
			return false, fmt.Errorf("%s port %d is already reserved by %s %s", protocol, port, existing.ResourceType, existing.ResourceID)
		}
		existing.UserID = userInfo.GetUserID()
		existing.TenantID = userInfo.GetTenantID()
		existing.UpdatedAt = time.Now()
		existing.ReleasedAt = nil
		activeKey := models.PortReservationActiveKey(nodeID, nodeType, protocol, port)
		existing.ActiveKey = &activeKey
		return false, tx.Save(&existing).Error
	}
	if err != gorm.ErrRecordNotFound {
		return false, err
	}

	activeKey := models.PortReservationActiveKey(nodeID, nodeType, protocol, port)
	reservation := &models.PortReservation{
		NodeID: nodeID, NodeType: nodeType, Protocol: protocol, Port: port,
		ActiveKey:    &activeKey,
		ResourceType: resourceType, ResourceID: resourceID,
		UserID: userInfo.GetUserID(), TenantID: userInfo.GetTenantID(),
		Status: models.PortReservationStatusActive,
	}
	if err := tx.Create(reservation).Error; err != nil {
		// A concurrent request may have won the unique index between the
		// lookup and insert. Keep the public error actionable.
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return false, fmt.Errorf("%s port %d is already reserved", protocol, port)
		}
		return false, err
	}
	return true, nil
}

func (m *portReservationMutation) ReleasePort(userInfo models.UserInfo, nodeID, nodeType, protocol string, port uint16, resourceType, resourceID string) error {
	if userInfo == nil || !userInfo.Valid() {
		return fmt.Errorf("user info is required")
	}
	nodeID = strings.TrimSpace(nodeID)
	nodeType = strings.TrimSpace(nodeType)
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	resourceType = strings.TrimSpace(resourceType)
	resourceID = strings.TrimSpace(resourceID)
	if nodeID == "" || nodeType == "" || protocol == "" || port == 0 || resourceType == "" || resourceID == "" {
		return fmt.Errorf("invalid port reservation")
	}
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	return db.Transaction(func(tx *gorm.DB) error {
		return releaseActivePortReservations(tx.Where(&models.PortReservation{
			NodeID: nodeID, NodeType: nodeType, Protocol: protocol, Port: port,
			ResourceType: resourceType, ResourceID: resourceID,
			UserID: userInfo.GetUserID(), TenantID: userInfo.GetTenantID(),
			Status: models.PortReservationStatusActive,
		}))
	})
}

// ReleaseProxyPortIfUnused releases a proxy reservation only when no active
// proxy record still resolves to the same logical resource. Shadow clients
// can represent one tunnel, so deleting one record must not unlock its port
// while another active record still uses it.
func (m *portReservationMutation) ReleaseProxyPortIfUnused(userInfo models.UserInfo, nodeID, nodeType, protocol string, port uint16, resourceID string) error {
	if userInfo == nil || !userInfo.Valid() {
		return fmt.Errorf("user info is required")
	}
	nodeID = strings.TrimSpace(nodeID)
	nodeType = strings.TrimSpace(nodeType)
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	resourceID = strings.TrimSpace(resourceID)
	if nodeID == "" || nodeType == "" || (protocol != "tcp" && protocol != "udp") || port == 0 || resourceID == "" {
		return fmt.Errorf("invalid proxy port reservation")
	}

	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	return db.Transaction(func(tx *gorm.DB) error {
		return releaseProxyReservationIfUnusedTx(tx, userInfo, proxyPortReservationRef{
			nodeID: nodeID, nodeType: nodeType, protocol: protocol, port: port, resourceID: resourceID,
		})
	})
}

// proxyPortReservationRef identifies one logical TCP/UDP tunnel reservation.
// The resource ID deliberately resolves Origin/Shadow clients to the same
// value, so release checks can protect a shared tunnel during client rebuilds.
type proxyPortReservationRef struct {
	nodeID     string
	nodeType   string
	protocol   string
	port       uint16
	resourceID string
}

// releaseProxyReservationIfUnusedTx must run after the caller has removed or
// replaced proxy rows, while still inside the caller's transaction. It only
// releases a reservation when no remaining active proxy resolves to the same
// logical resource.
func releaseProxyReservationIfUnusedTx(tx *gorm.DB, userInfo models.UserInfo, ref proxyPortReservationRef) error {
	if tx == nil {
		return fmt.Errorf("database transaction is required")
	}
	var reservation models.PortReservation
	err := tx.Where(&models.PortReservation{
		NodeID: ref.nodeID, NodeType: ref.nodeType, Protocol: ref.protocol, Port: ref.port,
		ResourceType: models.PortReservationTypeProxy, ResourceID: ref.resourceID,
		UserID: userInfo.GetUserID(), TenantID: userInfo.GetTenantID(),
		Status: models.PortReservationStatusActive,
	}).First(&reservation).Error
	if err == gorm.ErrRecordNotFound {
		return nil
	}
	if err != nil {
		return err
	}

	var proxies []*models.ProxyConfig
	if err := tx.Where("user_id = ? AND tenant_id = ? AND server_id = ?", userInfo.GetUserID(), userInfo.GetTenantID(), ref.nodeID).
		Where("(stopped IS NULL OR stopped = ?)", false).Find(&proxies).Error; err != nil {
		return err
	}
	for _, proxy := range proxies {
		proxyProtocol, proxyPort, ok := proxyRemotePortForReservation(proxy)
		if ok && proxyProtocol == ref.protocol && proxyPort == ref.port && proxyReservationIDForProxy(proxy) == ref.resourceID {
			return nil
		}
	}
	return releaseActivePortReservations(tx.Where("id = ?", reservation.ID))
}

func releaseProxyReservationRefsIfUnusedTx(tx *gorm.DB, userInfo models.UserInfo, refs []proxyPortReservationRef) error {
	seen := make(map[proxyPortReservationRef]struct{}, len(refs))
	for _, ref := range refs {
		if ref.nodeID == "" || ref.nodeType == "" || ref.resourceID == "" || ref.port == 0 || ref.protocol == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		if err := releaseProxyReservationIfUnusedTx(tx, userInfo, ref); err != nil {
			return err
		}
	}
	return nil
}

func (m *portReservationMutation) ReleaseResourcePorts(userInfo models.UserInfo, resourceType, resourceID string) error {
	if userInfo == nil || !userInfo.Valid() {
		return fmt.Errorf("user info is required")
	}
	resourceType = strings.TrimSpace(resourceType)
	resourceID = strings.TrimSpace(resourceID)
	if resourceType == "" || resourceID == "" {
		return fmt.Errorf("resource type and id are required")
	}
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	return db.Transaction(func(tx *gorm.DB) error {
		return releaseActivePortReservations(tx.Where("user_id = ? AND tenant_id = ? AND resource_type = ? AND resource_id = ? AND status = ?",
			userInfo.GetUserID(), userInfo.GetTenantID(), resourceType, resourceID, models.PortReservationStatusActive))
	})
}

func (m *portReservationMutation) ReleaseNodePorts(userInfo models.UserInfo, nodeID, nodeType string) error {
	if userInfo == nil || !userInfo.Valid() {
		return fmt.Errorf("user info is required")
	}
	nodeID = strings.TrimSpace(nodeID)
	nodeType = strings.TrimSpace(nodeType)
	if nodeID == "" || nodeType == "" {
		return fmt.Errorf("node id and node type are required")
	}
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	return db.Transaction(func(tx *gorm.DB) error {
		return releaseActivePortReservations(tx.Where("user_id = ? AND tenant_id = ? AND node_id = ? AND node_type = ? AND status = ?",
			userInfo.GetUserID(), userInfo.GetTenantID(), nodeID, nodeType, models.PortReservationStatusActive))
	})
}

func proxyRemotePortForReservation(proxy *models.ProxyConfig) (string, uint16, bool) {
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

func proxyReservationIDForProxy(proxy *models.ProxyConfig) string {
	if proxy == nil || proxy.ProxyConfigEntity == nil {
		return ""
	}
	return proxyReservationID(proxy.ServerID, proxy.ClientID, proxy.OriginClientID, proxy.Name)
}

func releaseActivePortReservations(query *gorm.DB) error {
	if query == nil {
		return fmt.Errorf("database query is required")
	}
	now := time.Now()
	return query.Model(&models.PortReservation{}).Updates(map[string]interface{}{
		"status":      models.PortReservationStatusReleased,
		"active_key":  nil,
		"released_at": now,
		"updated_at":  now,
	}).Error
}

// ReconcileServerPortReservations keeps the fixed listeners owned by frps and
// Caddy in the same physical-node reservation table as tunnel ports. Caddy's
// public 80/443 listeners are always reserved; gateway routes may share those
// two ports and therefore do not create a second exclusive reservation.
func (m *portReservationMutation) ReconcileServerPortReservations(userInfo models.UserInfo, serverID string, config *v1.ServerConfig) error {
	if userInfo == nil || !userInfo.Valid() {
		return fmt.Errorf("user info is required")
	}
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		return fmt.Errorf("server id is required")
	}
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	return db.Transaction(func(tx *gorm.DB) error {
		return reconcileServerPortReservationsTx(tx, userInfo, serverID, config)
	})
}

func reconcileServerPortReservationsTx(tx *gorm.DB, userInfo models.UserInfo, serverID string, config *v1.ServerConfig) error {
	if tx == nil {
		return fmt.Errorf("database transaction is required")
	}
	if err := releaseActivePortReservations(tx.Where("user_id = ? AND tenant_id = ? AND node_id = ? AND node_type = ? AND resource_type IN ? AND status = ?",
		userInfo.GetUserID(), userInfo.GetTenantID(), serverID, models.NodeTypeServer,
		[]string{models.PortReservationTypeFRPS, models.PortReservationTypeCaddy}, models.PortReservationStatusActive)); err != nil {
		return err
	}
	for _, item := range models.CaddyPortReservations() {
		resourceID := serverID + "/" + strings.TrimPrefix(item.Name, "caddy.")
		if _, err := reservePortTx(tx, userInfo, serverID, models.NodeTypeServer, item.Protocol, item.Port, models.PortReservationTypeCaddy, resourceID); err != nil {
			return err
		}
	}
	for _, item := range models.ServerPortReservations(config) {
		if _, err := reservePortTx(tx, userInfo, serverID, models.NodeTypeServer, item.Protocol, item.Port, models.PortReservationTypeFRPS, serverID+"/"+item.Name); err != nil {
			return err
		}
	}
	return nil
}
