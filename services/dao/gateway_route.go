package dao

import (
	"fmt"
	"strings"
	"time"

	"github.com/yunj-yun/frp-web-panel/models"
	"gorm.io/gorm"
)

type GatewayRouteQuery interface {
	ListGatewayRoutes(userInfo models.UserInfo, page, pageSize int, keyword, serverID string) ([]*models.GatewayRoute, error)
	CountGatewayRoutes(userInfo models.UserInfo, keyword, serverID string) (int64, error)
	GetGatewayRoute(userInfo models.UserInfo, id uint) (*models.GatewayRoute, error)
	ListGatewayRoutesByServer(userInfo models.UserInfo, serverID string) ([]*models.GatewayRoute, error)
	CountGatewayRoutesByCertificate(userInfo models.UserInfo, certificateID uint) (int64, error)
	FindGatewayRouteConflict(userInfo models.UserInfo, domain string, listenPort uint16, excludeID uint) (*models.GatewayRoute, error)
}

type GatewayRouteMutation interface {
	CreateGatewayRoute(userInfo models.UserInfo, route *models.GatewayRoute) error
	UpdateGatewayRoute(userInfo models.UserInfo, route *models.GatewayRoute) error
	DeleteGatewayRoute(userInfo models.UserInfo, id uint) error
	UpdateGatewayRouteApplyStatus(userInfo models.UserInfo, serverID string, status, errorMessage string, appliedAt *time.Time) error
}

type gatewayRouteQuery struct{ *queryImpl }
type gatewayRouteMutation struct{ *mutationImpl }

func newGatewayRouteQuery(base *queryImpl) GatewayRouteQuery { return &gatewayRouteQuery{base} }
func newGatewayRouteMutation(base *mutationImpl) GatewayRouteMutation {
	return &gatewayRouteMutation{base}
}

func gatewayRouteScope(userInfo models.UserInfo) *models.GatewayRoute {
	return &models.GatewayRoute{UserID: userInfo.GetUserID(), TenantID: userInfo.GetTenantID()}
}

func (q *gatewayRouteQuery) scoped(userInfo models.UserInfo) *gorm.DB {
	return q.ctx.GetApp().GetDBManager().GetDefaultDB().Where(gatewayRouteScope(userInfo))
}

func (q *gatewayRouteQuery) ListGatewayRoutes(userInfo models.UserInfo, page, pageSize int, keyword, serverID string) ([]*models.GatewayRoute, error) {
	if page < 1 || pageSize < 1 || pageSize > 100 {
		return nil, fmt.Errorf("invalid page or page size")
	}
	db := q.scoped(userInfo)
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		db = db.Where("domain LIKE ?", "%"+keyword+"%")
	}
	if serverID = strings.TrimSpace(serverID); serverID != "" {
		db = db.Where("server_id = ?", serverID)
	}
	var routes []*models.GatewayRoute
	err := db.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&routes).Error
	return routes, err
}

func (q *gatewayRouteQuery) CountGatewayRoutes(userInfo models.UserInfo, keyword, serverID string) (int64, error) {
	db := q.scoped(userInfo).Model(&models.GatewayRoute{})
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		db = db.Where("domain LIKE ?", "%"+keyword+"%")
	}
	if serverID = strings.TrimSpace(serverID); serverID != "" {
		db = db.Where("server_id = ?", serverID)
	}
	var count int64
	err := db.Count(&count).Error
	return count, err
}

func (q *gatewayRouteQuery) GetGatewayRoute(userInfo models.UserInfo, id uint) (*models.GatewayRoute, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid gateway route id")
	}
	route := &models.GatewayRoute{}
	if err := q.scoped(userInfo).First(route, id).Error; err != nil {
		return nil, err
	}
	return route, nil
}

func (q *gatewayRouteQuery) ListGatewayRoutesByServer(userInfo models.UserInfo, serverID string) ([]*models.GatewayRoute, error) {
	if strings.TrimSpace(serverID) == "" {
		return nil, fmt.Errorf("server id is required")
	}
	var routes []*models.GatewayRoute
	err := q.scoped(userInfo).Where("server_id = ?", serverID).Order("domain ASC, listen_port ASC").Find(&routes).Error
	return routes, err
}

func (q *gatewayRouteQuery) CountGatewayRoutesByCertificate(userInfo models.UserInfo, certificateID uint) (int64, error) {
	if certificateID == 0 {
		return 0, fmt.Errorf("invalid certificate id")
	}
	var count int64
	err := q.scoped(userInfo).Model(&models.GatewayRoute{}).Where("certificate_id = ?", certificateID).Count(&count).Error
	return count, err
}

func (q *gatewayRouteQuery) FindGatewayRouteConflict(userInfo models.UserInfo, domain string, listenPort uint16, excludeID uint) (*models.GatewayRoute, error) {
	if strings.TrimSpace(domain) == "" || listenPort == 0 {
		return nil, fmt.Errorf("domain and listen port are required")
	}
	db := q.scoped(userInfo).Where("LOWER(domain) = ? AND listen_port = ?", strings.ToLower(strings.TrimSpace(domain)), listenPort)
	if excludeID != 0 {
		db = db.Where("id <> ?", excludeID)
	}
	route := &models.GatewayRoute{}
	err := db.First(route).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return route, err
}

func (m *gatewayRouteMutation) CreateGatewayRoute(userInfo models.UserInfo, route *models.GatewayRoute) error {
	if route == nil {
		return fmt.Errorf("gateway route is required")
	}
	route.UserID = userInfo.GetUserID()
	route.TenantID = userInfo.GetTenantID()
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(route).Error; err != nil {
			return err
		}
		return reserveGatewayRoutePort(tx, userInfo, route)
	})
}

func (m *gatewayRouteMutation) UpdateGatewayRoute(userInfo models.UserInfo, route *models.GatewayRoute) error {
	if route == nil || route.ID == 0 {
		return fmt.Errorf("gateway route is required")
	}
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	return db.Transaction(func(tx *gorm.DB) error {
		var existing models.GatewayRoute
		if err := tx.Where(gatewayRouteScope(userInfo)).First(&existing, route.ID).Error; err != nil {
			return err
		}
		oldPort := existing.ListenPort
		oldServerID := existing.ServerID
		existing.Domain = route.Domain
		existing.ListenPort = route.ListenPort
		existing.ServerID = route.ServerID
		existing.ProxyID = route.ProxyID
		existing.CertificateID = route.CertificateID
		existing.UpstreamPort = route.UpstreamPort
		existing.Status = route.Status
		existing.ErrorMessage = route.ErrorMessage
		existing.LastAppliedAt = route.LastAppliedAt
		if err := tx.Save(&existing).Error; err != nil {
			return err
		}
		if oldPort != route.ListenPort || oldServerID != route.ServerID {
			if err := reconcileGatewayRoutePortReservationTx(tx, userInfo, oldServerID, oldPort); err != nil {
				return err
			}
		}
		return reserveGatewayRoutePort(tx, userInfo, &existing)
	})
}

func (m *gatewayRouteMutation) DeleteGatewayRoute(userInfo models.UserInfo, id uint) error {
	if id == 0 {
		return fmt.Errorf("invalid gateway route id")
	}
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	return db.Transaction(func(tx *gorm.DB) error {
		var route models.GatewayRoute
		if err := tx.Where(gatewayRouteScope(userInfo)).First(&route, id).Error; err != nil {
			return err
		}
		if err := tx.Delete(&route).Error; err != nil {
			return err
		}
		return reconcileGatewayRoutePortReservationTx(tx, userInfo, route.ServerID, route.ListenPort)
	})
}

func reserveGatewayRoutePort(tx *gorm.DB, userInfo models.UserInfo, route *models.GatewayRoute) error {
	if tx == nil || userInfo == nil || route == nil || route.ListenPort == 0 || strings.TrimSpace(route.ServerID) == "" {
		return fmt.Errorf("invalid gateway route port reservation")
	}
	if route.ListenPort == models.CaddyHTTPPort || route.ListenPort == models.CaddyHTTPSPort {
		return nil
	}
	resourceID := models.GatewayRoutePortResourceID(route.ServerID, route.ListenPort)
	var existing models.PortReservation
	err := tx.Where(&models.PortReservation{
		NodeID: route.ServerID, NodeType: models.NodeTypeServer, Protocol: "tcp", Port: route.ListenPort,
		Status: models.PortReservationStatusActive,
	}).First(&existing).Error
	if err == nil && existing.ResourceType == models.PortReservationTypeGateway && existing.ResourceID != resourceID && strings.HasPrefix(existing.ResourceID, "route/") {
		return tx.Model(&models.PortReservation{}).Where("id = ?", existing.ID).Updates(map[string]interface{}{
			"resource_id": resourceID,
			"user_id":     userInfo.GetUserID(),
			"tenant_id":   userInfo.GetTenantID(),
			"updated_at":  time.Now(),
		}).Error
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	_, err = reservePortTx(tx, userInfo, route.ServerID, models.NodeTypeServer, "tcp", route.ListenPort,
		models.PortReservationTypeGateway, resourceID)
	return err
}

func reconcileGatewayRoutePortReservationTx(tx *gorm.DB, userInfo models.UserInfo, serverID string, port uint16) error {
	if tx == nil || userInfo == nil || strings.TrimSpace(serverID) == "" || port == 0 {
		return fmt.Errorf("invalid gateway route port reservation")
	}
	if port == models.CaddyHTTPPort || port == models.CaddyHTTPSPort {
		return nil
	}
	var routeCount int64
	if err := tx.Model(&models.GatewayRoute{}).Where("server_id = ? AND listen_port = ?", serverID, port).Count(&routeCount).Error; err != nil {
		return err
	}
	if routeCount == 0 {
		return releaseActivePortReservations(tx.Where("node_id = ? AND node_type = ? AND protocol = ? AND port = ? AND resource_type = ? AND status = ?",
			serverID, models.NodeTypeServer, "tcp", port, models.PortReservationTypeGateway, models.PortReservationStatusActive))
	}
	return reserveGatewayRoutePort(tx, userInfo, &models.GatewayRoute{ServerID: serverID, ListenPort: port})
}

// UpdateGatewayRouteApplyStatus updates the result of one complete Caddy
// configuration application. All routes for the target Server share the same
// desired document, therefore the Agent result is reflected consistently on
// every route in that document.
func (m *gatewayRouteMutation) UpdateGatewayRouteApplyStatus(userInfo models.UserInfo, serverID string, routeStatus, errorMessage string, appliedAt *time.Time) error {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		return fmt.Errorf("server id is required")
	}
	if routeStatus != models.GatewayRouteStatusActive && routeStatus != models.GatewayRouteStatusError {
		return fmt.Errorf("invalid gateway route status")
	}
	updates := map[string]interface{}{
		"status":        routeStatus,
		"error_message": strings.TrimSpace(errorMessage),
	}
	if appliedAt != nil {
		updates["last_applied_at"] = appliedAt
	}
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB().Where(gatewayRouteScope(userInfo)).Where("server_id = ?", serverID)
	return db.Model(&models.GatewayRoute{}).Updates(updates).Error
}
