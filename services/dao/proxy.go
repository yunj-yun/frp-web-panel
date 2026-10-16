package dao

import (
	"fmt"
	"strings"
	"time"

	"github.com/yunj-yun/frp-web-panel/models"
	"github.com/yunj-yun/frp-web-panel/pb"
	"github.com/yunj-yun/frp-web-panel/utils"
	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProxyQuery interface {
	GetProxyStatsByClientID(userInfo models.UserInfo, clientID string) ([]*models.ProxyStatsEntity, error)
	GetProxyStatsByServerID(userInfo models.UserInfo, serverID string) ([]*models.ProxyStatsEntity, error)
	AdminGetTenantProxyStats(tenantID int) ([]*models.ProxyStatsEntity, error)
	AdminGetAllProxyStats(tx *gorm.DB) ([]*models.ProxyStatsEntity, error)
	AdminGetProxyConfigByClientIDAndName(clientID string, name string) (*models.ProxyConfig, error)
	GetProxyConfigByID(userInfo models.UserInfo, id uint) (*models.ProxyConfig, error)
	GetProxyConfigsByClientID(userInfo models.UserInfo, clientID string) ([]*models.ProxyConfigEntity, error)
	ListProxyConfigsByServerID(userInfo models.UserInfo, serverID string) ([]*models.ProxyConfig, error)
	GetProxyConfigByFilter(userInfo models.UserInfo, proxyConfig *models.ProxyConfigEntity) (*models.ProxyConfig, error)
	ListProxyConfigsWithFilters(userInfo models.UserInfo, page, pageSize int, filters *models.ProxyConfigEntity) ([]*models.ProxyConfig, error)
	AdminListProxyConfigsWithFilters(filters *models.ProxyConfigEntity) ([]*models.ProxyConfig, error)
	ListProxyConfigsWithFiltersAndKeyword(userInfo models.UserInfo, page, pageSize int, filters *models.ProxyConfigEntity, keyword string) ([]*models.ProxyConfig, error)
	ListProxyConfigsWithKeyword(userInfo models.UserInfo, page, pageSize int, keyword string) ([]*models.ProxyConfig, error)
	ListProxyConfigs(userInfo models.UserInfo, page, pageSize int) ([]*models.ProxyConfig, error)
	GetProxyConfigByOriginClientIDAndName(userInfo models.UserInfo, clientID string, name string) (*models.ProxyConfig, error)
	CountProxyConfigs(userInfo models.UserInfo) (int64, error)
	CountProxyConfigsWithFilters(userInfo models.UserInfo, filters *models.ProxyConfigEntity) (int64, error)
	CountProxyConfigsWithFiltersAndKeyword(userInfo models.UserInfo, filters *models.ProxyConfigEntity, keyword string) (int64, error)
	GetProxyConfigsByWorkerId(userInfo models.UserInfo, workerID string) ([]*models.ProxyConfig, error)
}

type ProxyMutation interface {
	AdminUpdateProxyStats(srv *models.ServerEntity, inputs []*pb.ProxyInfo) error
	AdminCreateProxyConfig(proxyCfg *models.ProxyConfig) error
	RebuildProxyConfigFromClient(userInfo models.UserInfo, client *models.Client) error
	CreateProxyConfig(userInfo models.UserInfo, proxyCfg *models.ProxyConfigEntity) error
	UpdateProxyConfig(userInfo models.UserInfo, proxyCfg *models.ProxyConfig) error
	DeleteProxyConfig(userInfo models.UserInfo, clientID, name string) error
	DeleteProxyConfigsByClientIDOrOriginClientID(userInfo models.UserInfo, clientID string) error
	DeleteProxyConfigsByClientID(userInfo models.UserInfo, clientID string) error
}

type proxyQuery struct{ *queryImpl }
type proxyMutation struct{ *mutationImpl }

func newProxyQuery(base *queryImpl) ProxyQuery          { return &proxyQuery{base} }
func newProxyMutation(base *mutationImpl) ProxyMutation { return &proxyMutation{base} }

func (q *proxyQuery) GetProxyStatsByClientID(userInfo models.UserInfo, clientID string) ([]*models.ProxyStatsEntity, error) {
	if clientID == "" {
		return nil, fmt.Errorf("invalid client id")
	}
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	list := []*models.ProxyStats{}
	err := db.
		Where(&models.ProxyStats{ProxyStatsEntity: &models.ProxyStatsEntity{
			UserID:   userInfo.GetUserID(),
			TenantID: userInfo.GetTenantID(),
			ClientID: clientID,
		}}).
		Or(&models.ProxyStats{ProxyStatsEntity: &models.ProxyStatsEntity{
			UserID:   0,
			TenantID: userInfo.GetTenantID(),
			ClientID: clientID,
		}}).
		Or(&models.ProxyStats{ProxyStatsEntity: &models.ProxyStatsEntity{
			UserID:         userInfo.GetUserID(),
			TenantID:       userInfo.GetTenantID(),
			OriginClientID: clientID,
		}}).
		Or(&models.ProxyStats{ProxyStatsEntity: &models.ProxyStatsEntity{
			UserID:         0,
			TenantID:       userInfo.GetTenantID(),
			OriginClientID: clientID,
		}}).
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return lo.Map(list, func(item *models.ProxyStats, _ int) *models.ProxyStatsEntity {
		return item.ProxyStatsEntity
	}), nil
}

func (q *proxyQuery) GetProxyStatsByServerID(userInfo models.UserInfo, serverID string) ([]*models.ProxyStatsEntity, error) {
	if serverID == "" {
		return nil, fmt.Errorf("invalid server id")
	}
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	list := []*models.ProxyStats{}
	err := db.
		Where(&models.ProxyStats{ProxyStatsEntity: &models.ProxyStatsEntity{
			UserID:   userInfo.GetUserID(),
			TenantID: userInfo.GetTenantID(),
			ServerID: serverID,
		}}).Or(&models.ProxyStats{ProxyStatsEntity: &models.ProxyStatsEntity{
		UserID:   0,
		TenantID: userInfo.GetTenantID(),
		ServerID: serverID,
	}}).
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return lo.Map(list, func(item *models.ProxyStats, _ int) *models.ProxyStatsEntity {
		return item.ProxyStatsEntity
	}), nil
}

func (m *proxyMutation) AdminUpdateProxyStats(srv *models.ServerEntity, inputs []*pb.ProxyInfo) error {
	if srv.ServerID == "" {
		return fmt.Errorf("invalid server id")
	}

	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	return db.Transaction(func(tx *gorm.DB) error {

		queryResults := make([]interface{}, 3)
		p := pool.New().WithErrors()
		p.Go(
			func() error {
				user := models.User{}
				if err := tx.Where(&models.User{
					UserEntity: &models.UserEntity{
						UserID: srv.UserID,
					},
				}).First(&user).Error; err != nil {
					return err
				}
				queryResults[0] = user
				return nil
			},
		)
		p.Go(
			func() error {
				clients := []*models.Client{}
				if err := tx.
					Where(&models.Client{ClientEntity: &models.ClientEntity{
						UserID:   srv.UserID,
						ServerID: srv.ServerID,
					}}).Find(&clients).Error; err != nil {
					return err
				}
				queryResults[1] = clients
				return nil
			},
		)
		p.Go(
			func() error {
				oldProxy := []*models.ProxyStats{}
				if err := tx.
					Where(&models.ProxyStats{ProxyStatsEntity: &models.ProxyStatsEntity{
						UserID:   srv.UserID,
						ServerID: srv.ServerID,
					}}).Find(&oldProxy).Error; err != nil {
					return err
				}
				oldProxyMap := lo.SliceToMap(oldProxy, func(p *models.ProxyStats) (string, *models.ProxyStats) {
					return p.Name, p
				})
				queryResults[2] = oldProxyMap
				return nil
			},
		)
		if err := p.Wait(); err != nil {
			return err
		}

		user := queryResults[0].(models.User)
		clients := queryResults[1].([]*models.Client)
		oldProxyMap := queryResults[2].(map[string]*models.ProxyStats)

		inputMap := map[string]*pb.ProxyInfo{}
		proxyMap := map[string]*models.ProxyStatsEntity{}
		for _, proxyInfo := range inputs {
			if proxyInfo == nil {
				continue
			}
			proxyName := strings.TrimPrefix(proxyInfo.GetName(), user.UserName+".")
			proxyMap[proxyName] = &models.ProxyStatsEntity{
				ServerID:        srv.ServerID,
				Name:            proxyName,
				Type:            proxyInfo.GetType(),
				UserID:          srv.UserID,
				TenantID:        srv.TenantID,
				TodayTrafficIn:  proxyInfo.GetTodayTrafficIn(),
				TodayTrafficOut: proxyInfo.GetTodayTrafficOut(),
			}
			inputMap[proxyName] = proxyInfo
		}

		proxyEntityMap := map[string]*models.ProxyStatsEntity{}
		for _, client := range clients {
			cliCfg, err := client.GetConfigContent()
			if err != nil || cliCfg == nil {
				continue
			}
			for _, cfg := range cliCfg.Proxies {
				if proxy, ok := proxyMap[cfg.GetBaseConfig().Name]; ok {
					proxy.ClientID = client.ClientID
					proxy.OriginClientID = client.OriginClientID
					proxyEntityMap[proxy.Name] = proxy
				}
			}
		}

		nowTime := time.Now()
		results := lo.Values(lo.MapValues(proxyEntityMap, func(p *models.ProxyStatsEntity, name string) *models.ProxyStats {
			item := &models.ProxyStats{
				ProxyStatsEntity: p,
			}
			if oldProxy, ok := oldProxyMap[name]; ok {
				item.ProxyID = oldProxy.ProxyID
				firstSync := inputMap[name].GetFirstSync()
				isSameDay := utils.IsSameDay(nowTime, oldProxy.UpdatedAt)

				item.HistoryTrafficIn = oldProxy.HistoryTrafficIn
				item.HistoryTrafficOut = oldProxy.HistoryTrafficOut
				if !isSameDay || firstSync {
					item.HistoryTrafficIn += oldProxy.TodayTrafficIn
					item.HistoryTrafficOut += oldProxy.TodayTrafficOut
				}
			}
			return item
		}))

		if len(results) > 0 {
			return tx.Save(results).Error
		}
		return nil
	})
}

func (q *proxyQuery) AdminGetTenantProxyStats(tenantID int) ([]*models.ProxyStatsEntity, error) {
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	list := []*models.ProxyStats{}
	err := db.
		Where(&models.ProxyStats{ProxyStatsEntity: &models.ProxyStatsEntity{
			TenantID: tenantID,
		}}).
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return lo.Map(list, func(item *models.ProxyStats, _ int) *models.ProxyStatsEntity {
		return item.ProxyStatsEntity
	}), nil
}

func (q *proxyQuery) AdminGetAllProxyStats(tx *gorm.DB) ([]*models.ProxyStatsEntity, error) {
	db := tx
	list := []*models.ProxyStats{}
	err := db.Clauses(clause.Locking{Strength: "UPDATE"}).
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return lo.Map(list, func(item *models.ProxyStats, _ int) *models.ProxyStatsEntity {
		return item.ProxyStatsEntity
	}), nil
}

func (m *proxyMutation) AdminCreateProxyConfig(proxyCfg *models.ProxyConfig) error {
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	return db.Create(proxyCfg).Error
}

// RebuildProxyConfigFromClient rebuild proxy from client
// skip stopped proxy
func (m *proxyMutation) RebuildProxyConfigFromClient(userInfo models.UserInfo, client *models.Client) error {
	if userInfo == nil || !userInfo.Valid() || client == nil || client.ClientEntity == nil {
		return fmt.Errorf("invalid user or client")
	}
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()

	pxyCfgs, err := utils.LoadProxiesFromContent(client.ConfigContent)
	if err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// Capture the existing rows before replacing active rows. Reservations
		// are released only after deletion, and only when no remaining Shadow
		// Client still resolves to the same logical tunnel.
		var oldConfigs []*models.ProxyConfig
		if err := tx.Where("user_id = ? AND tenant_id = ? AND client_id = ?",
			userInfo.GetUserID(), userInfo.GetTenantID(), client.ClientID).Find(&oldConfigs).Error; err != nil {
			return err
		}
		oldRefs := make([]proxyPortReservationRef, 0, len(oldConfigs))
		for _, old := range oldConfigs {
			if old != nil && old.ProxyConfigEntity != nil {
				if protocol, port, ok := proxyRemotePortForReservation(old); ok {
					oldRefs = append(oldRefs, proxyPortReservationRef{
						nodeID: old.ServerID, nodeType: models.NodeTypeServer, protocol: protocol,
						port: port, resourceID: proxyReservationIDForProxy(old),
					})
				}
			}
		}

		proxyConfigEntities := make([]*models.ProxyConfig, 0, len(pxyCfgs))
		for _, pxyCfg := range pxyCfgs {
			if pxyCfg.GetBaseConfig() == nil || pxyCfg.GetBaseConfig().Name == "" {
				return fmt.Errorf("proxy configuration name is required")
			}
			proxyCfg := &models.ProxyConfig{ProxyConfigEntity: &models.ProxyConfigEntity{}}
			for _, old := range oldConfigs {
				if old != nil && old.Name == pxyCfg.GetBaseConfig().Name && old.Model != nil {
					proxyCfg.Model = old.Model
					break
				}
			}
			if err := proxyCfg.FillClientConfig(client.ClientEntity); err != nil {
				return err
			}
			if err := proxyCfg.FillTypedProxyConfig(pxyCfg); err != nil {
				return err
			}
			if err := reserveRebuiltProxyPort(tx, userInfo, client, pxyCfg, oldConfigs); err != nil {
				return err
			}
			proxyConfigEntities = append(proxyConfigEntities, proxyCfg)
		}

		// Rebuild historically preserved stopped rows. They are not in the
		// active client config and therefore do not receive reservations.
		if err := tx.Unscoped().Where("user_id = ? AND tenant_id = ? AND client_id = ?",
			userInfo.GetUserID(), userInfo.GetTenantID(), client.ClientID).
			Where("stopped IS NULL OR stopped = ?", false).Delete(&models.ProxyConfig{}).Error; err != nil {
			return err
		}
		if len(proxyConfigEntities) == 0 {
			return releaseProxyReservationRefsIfUnusedTx(tx, userInfo, oldRefs)
		}
		if err := tx.Save(proxyConfigEntities).Error; err != nil {
			return err
		}
		return releaseProxyReservationRefsIfUnusedTx(tx, userInfo, oldRefs)
	})
}

func reserveRebuiltProxyPort(tx *gorm.DB, userInfo models.UserInfo, client *models.Client, cfg v1.TypedProxyConfig, oldConfigs []*models.ProxyConfig) error {
	protocol, port, ok := rebuiltProxyRemotePort(cfg)
	if !ok || client.ServerID == "" {
		return nil
	}
	var server models.Server
	if err := tx.Where("server_id = ? AND user_id = ? AND tenant_id = ?", client.ServerID, userInfo.GetUserID(), userInfo.GetTenantID()).First(&server).Error; err != nil {
		return fmt.Errorf("get server for proxy port validation: %w", err)
	}
	serverConfig, err := server.GetConfigContent()
	if err != nil || serverConfig == nil {
		if err == nil {
			err = fmt.Errorf("empty server configuration")
		}
		return fmt.Errorf("read server configuration for proxy port validation: %w", err)
	}
	if reserved := rebuiltServerPortName(port, serverConfig); reserved != "" {
		return fmt.Errorf("%s proxy remote port %d conflicts with %s", protocol, port, reserved)
	}

	originClientID := client.OriginClientID
	if originClientID == "" {
		originClientID = client.ClientID
	}
	resourceID := proxyReservationID(client.ServerID, client.ClientID, client.OriginClientID, cfg.GetBaseConfig().Name)
	aliases := make([]string, 0, 2)
	for _, old := range oldConfigs {
		if old == nil || old.ServerID != client.ServerID {
			continue
		}
		if old.ClientID == client.ClientID || old.OriginClientID == originClientID {
			aliases = append(aliases, proxyReservationID(old.ServerID, old.ClientID, old.OriginClientID, old.Name))
		}
	}
	if _, err := reservePortTx(tx, userInfo, client.ServerID, "server", protocol, port, models.PortReservationTypeProxy, resourceID, aliases...); err != nil {
		return fmt.Errorf("reserve %s proxy remote port %d: %w", protocol, port, err)
	}

	// The reservation table may predate this rebuild. Check active persisted
	// proxy rows as a second source of truth, excluding this client's records.
	var existing []*models.ProxyConfig
	if err := tx.Where("user_id = ? AND tenant_id = ? AND server_id = ? AND (stopped IS NULL OR stopped = ?)",
		userInfo.GetUserID(), userInfo.GetTenantID(), client.ServerID, false).Find(&existing).Error; err != nil {
		return err
	}
	for _, item := range existing {
		if item == nil || item.ClientID == client.ClientID || item.OriginClientID == originClientID {
			continue
		}
		existingCfg, err := item.GetTypedProxyConfig()
		if err != nil {
			return fmt.Errorf("read existing proxy %q for port validation: %w", item.Name, err)
		}
		existingProtocol, existingPort, existingOK := rebuiltProxyRemotePort(existingCfg)
		if existingOK && existingProtocol == protocol && existingPort == port {
			return fmt.Errorf("%s proxy remote port %d is already assigned to proxy %q", protocol, port, item.Name)
		}
	}
	return nil
}

func rebuiltProxyRemotePort(cfg v1.TypedProxyConfig) (string, uint16, bool) {
	if cfg.ProxyConfigurer == nil || cfg.GetBaseConfig() == nil {
		return "", 0, false
	}
	protocol := cfg.GetBaseConfig().Type
	var port int
	switch typed := cfg.ProxyConfigurer.(type) {
	case *v1.TCPProxyConfig:
		protocol, port = string(v1.ProxyTypeTCP), typed.RemotePort
	case *v1.UDPProxyConfig:
		protocol, port = string(v1.ProxyTypeUDP), typed.RemotePort
	default:
		return "", 0, false
	}
	if port <= 0 || port > 65535 {
		return protocol, 0, false
	}
	return protocol, uint16(port), true
}

func rebuiltServerPortName(port uint16, config *v1.ServerConfig) string {
	if config == nil {
		return ""
	}
	reserved := []struct {
		port uint32
		name string
	}{
		{uint32(config.BindPort), "frps bindPort"},
		{uint32(config.KCPBindPort), "frps kcpBindPort"},
		{uint32(config.QUICBindPort), "frps quicBindPort"},
		{uint32(config.VhostHTTPPort), "frps vhostHTTPPort"},
		{uint32(config.VhostHTTPSPort), "frps vhostHTTPSPort"},
		{uint32(config.TCPMuxHTTPConnectPort), "frps tcpmuxHTTPConnectPort"},
		{uint32(config.WebServer.Port), "frps webServer.port"},
		{uint32(config.SSHTunnelGateway.BindPort), "frps sshTunnelGateway.bindPort"},
	}
	for _, item := range reserved {
		if item.port != 0 && item.port == uint32(port) {
			return item.name
		}
	}
	return ""
}

func proxyReservationID(serverID, clientID, originClientID, proxyName string) string {
	owner := strings.TrimSpace(originClientID)
	if owner == "" {
		owner = strings.TrimSpace(clientID)
	}
	return fmt.Sprintf("%s/%s/%s", strings.TrimSpace(serverID), owner, strings.TrimSpace(proxyName))
}

func (q *proxyQuery) AdminGetProxyConfigByClientIDAndName(clientID string, name string) (*models.ProxyConfig, error) {
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	proxyCfg := &models.ProxyConfig{}
	err := db.
		Where(&models.ProxyConfig{ProxyConfigEntity: &models.ProxyConfigEntity{
			ClientID: clientID,
			Name:     name,
		}}).
		First(proxyCfg).Error
	if err != nil {
		return nil, err
	}
	return proxyCfg, nil
}

// GetProxyConfigByID returns a proxy configuration visible to the current
// user and tenant.  Gateway routes refer to proxy records by their database
// ID, so the ownership predicates must be part of the query rather than a
// separate check after loading the record.
func (q *proxyQuery) GetProxyConfigByID(userInfo models.UserInfo, id uint) (*models.ProxyConfig, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid proxy config id")
	}
	proxyCfg := &models.ProxyConfig{}
	err := q.ctx.GetApp().GetDBManager().GetDefaultDB().Where(&models.ProxyConfig{
		Model: &gorm.Model{ID: id},
		ProxyConfigEntity: &models.ProxyConfigEntity{
			UserID:   userInfo.GetUserID(),
			TenantID: userInfo.GetTenantID(),
		},
	}).First(proxyCfg).Error
	if err != nil {
		return nil, err
	}
	return proxyCfg, nil
}

func (q *proxyQuery) GetProxyConfigsByClientID(userInfo models.UserInfo, clientID string) ([]*models.ProxyConfigEntity, error) {
	if clientID == "" {
		return nil, fmt.Errorf("invalid client id")
	}
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	list := []*models.ProxyConfig{}
	err := db.
		Where(&models.ProxyConfig{ProxyConfigEntity: &models.ProxyConfigEntity{
			UserID:   userInfo.GetUserID(),
			TenantID: userInfo.GetTenantID(),
			ClientID: clientID,
		}}).
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return lo.Map(list, func(item *models.ProxyConfig, _ int) *models.ProxyConfigEntity {
		return item.ProxyConfigEntity
	}), nil
}

// ListProxyConfigsByServerID returns all proxy records belonging to a server
// that are visible to the current user.  Unlike the UI list query this method
// intentionally has no pagination: it is used for authoritative port
// conflict checks before a new FRP configuration is applied.
func (q *proxyQuery) ListProxyConfigsByServerID(userInfo models.UserInfo, serverID string) ([]*models.ProxyConfig, error) {
	if userInfo == nil || serverID == "" {
		return nil, fmt.Errorf("invalid user or server id")
	}

	var proxyConfigs []*models.ProxyConfig
	err := q.ctx.GetApp().GetDBManager().GetDefaultDB().Where(&models.ProxyConfig{
		ProxyConfigEntity: &models.ProxyConfigEntity{
			UserID:   userInfo.GetUserID(),
			TenantID: userInfo.GetTenantID(),
			ServerID: serverID,
		},
	}).Order("id ASC").Find(&proxyConfigs).Error
	if err != nil {
		return nil, err
	}
	return proxyConfigs, nil
}

func (q *proxyQuery) GetProxyConfigByFilter(userInfo models.UserInfo, proxyConfig *models.ProxyConfigEntity) (*models.ProxyConfig, error) {
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	filter := &models.ProxyConfigEntity{}

	if len(proxyConfig.ClientID) != 0 {
		filter.ClientID = proxyConfig.ClientID
	}
	if len(proxyConfig.OriginClientID) != 0 {
		filter.OriginClientID = proxyConfig.OriginClientID
	}
	if len(proxyConfig.Name) != 0 {
		filter.Name = proxyConfig.Name
	}
	if len(proxyConfig.Type) != 0 {
		filter.Type = proxyConfig.Type
	}
	if len(proxyConfig.ServerID) != 0 {
		filter.ServerID = proxyConfig.ServerID
	}

	filter.UserID = userInfo.GetUserID()
	filter.TenantID = userInfo.GetTenantID()

	respProxyCfg := &models.ProxyConfig{}
	err := db.
		Where(&models.ProxyConfig{ProxyConfigEntity: filter}).
		First(respProxyCfg).Error
	if err != nil {
		return nil, err
	}
	return respProxyCfg, nil
}

func (q *proxyQuery) ListProxyConfigsWithFilters(userInfo models.UserInfo, page, pageSize int, filters *models.ProxyConfigEntity) ([]*models.ProxyConfig, error) {
	if page < 1 || pageSize < 1 {
		return nil, fmt.Errorf("invalid page or page size")
	}

	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	offset := (page - 1) * pageSize

	filters.UserID = userInfo.GetUserID()
	filters.TenantID = userInfo.GetTenantID()

	var proxyConfigs []*models.ProxyConfig
	err := db.Where(&models.ProxyConfig{
		ProxyConfigEntity: filters,
	}).Where(filters).Offset(offset).Limit(pageSize).Find(&proxyConfigs).Error
	if err != nil {
		return nil, err
	}

	return proxyConfigs, nil
}

func (q *proxyQuery) AdminListProxyConfigsWithFilters(filters *models.ProxyConfigEntity) ([]*models.ProxyConfig, error) {
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()

	var proxyConfigs []*models.ProxyConfig
	err := db.Where(&models.ProxyConfig{
		ProxyConfigEntity: filters,
	}).Where(filters).Find(&proxyConfigs).Error
	if err != nil {
		return nil, err
	}

	return proxyConfigs, nil
}

func (q *proxyQuery) ListProxyConfigsWithFiltersAndKeyword(userInfo models.UserInfo, page, pageSize int, filters *models.ProxyConfigEntity, keyword string) ([]*models.ProxyConfig, error) {
	if page < 1 || pageSize < 1 || len(keyword) == 0 {
		return nil, fmt.Errorf("invalid page or page size or keyword")
	}

	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	offset := (page - 1) * pageSize

	filters.UserID = userInfo.GetUserID()
	filters.TenantID = userInfo.GetTenantID()

	var proxyConfigs []*models.ProxyConfig
	err := db.Where(&models.ProxyConfig{
		ProxyConfigEntity: filters,
	}).Where(filters).Where("name like ?", "%"+keyword+"%").Offset(offset).Limit(pageSize).Find(&proxyConfigs).Error
	if err != nil {
		return nil, err
	}

	return proxyConfigs, nil
}

func (q *proxyQuery) ListProxyConfigsWithKeyword(userInfo models.UserInfo, page, pageSize int, keyword string) ([]*models.ProxyConfig, error) {
	return q.ListProxyConfigsWithFiltersAndKeyword(userInfo, page, pageSize, &models.ProxyConfigEntity{}, keyword)
}

func (q *proxyQuery) ListProxyConfigs(userInfo models.UserInfo, page, pageSize int) ([]*models.ProxyConfig, error) {
	return q.ListProxyConfigsWithFilters(userInfo, page, pageSize, &models.ProxyConfigEntity{})
}

func (m *proxyMutation) CreateProxyConfig(userInfo models.UserInfo, proxyCfg *models.ProxyConfigEntity) error {
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	proxyCfg.UserID = userInfo.GetUserID()
	proxyCfg.TenantID = userInfo.GetTenantID()
	return db.Create(&models.ProxyConfig{ProxyConfigEntity: proxyCfg}).Error
}

func (m *proxyMutation) UpdateProxyConfig(userInfo models.UserInfo, proxyCfg *models.ProxyConfig) error {
	if proxyCfg.ID == 0 {
		return fmt.Errorf("invalid proxy config id")
	}
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	proxyCfg.UserID = userInfo.GetUserID()
	proxyCfg.TenantID = userInfo.GetTenantID()
	return db.Where(&models.ProxyConfig{
		ProxyConfigEntity: &models.ProxyConfigEntity{
			UserID:   userInfo.GetUserID(),
			TenantID: userInfo.GetTenantID(),
			ClientID: proxyCfg.ClientID,
		},
		Model: &gorm.Model{
			ID: proxyCfg.ID,
		},
	}).Save(proxyCfg).Error
}

func (m *proxyMutation) DeleteProxyConfig(userInfo models.UserInfo, clientID, name string) error {
	if clientID == "" || name == "" {
		return fmt.Errorf("invalid client id or name")
	}
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	return db.Unscoped().
		Where(&models.ProxyConfig{ProxyConfigEntity: &models.ProxyConfigEntity{
			UserID:   userInfo.GetUserID(),
			TenantID: userInfo.GetTenantID(),
			ClientID: clientID,
			Name:     name,
		}}).
		Delete(&models.ProxyConfig{}).Error
}

func (m *proxyMutation) DeleteProxyConfigsByClientIDOrOriginClientID(userInfo models.UserInfo, clientID string) error {
	if userInfo == nil || !userInfo.Valid() || clientID == "" {
		return fmt.Errorf("invalid client id")
	}
	return m.deleteProxyConfigs(userInfo, clientID, true)
}

func (m *proxyMutation) DeleteProxyConfigsByClientID(userInfo models.UserInfo, clientID string) error {
	if userInfo == nil || !userInfo.Valid() || clientID == "" {
		return fmt.Errorf("invalid client id")
	}
	return m.deleteProxyConfigs(userInfo, clientID, false)
}

func (m *proxyMutation) deleteProxyConfigs(userInfo models.UserInfo, clientID string, includeOrigin bool) error {
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	return db.Transaction(func(tx *gorm.DB) error {
		base := tx.Where("user_id = ? AND tenant_id = ?", userInfo.GetUserID(), userInfo.GetTenantID()).
			Where("(stopped IS NULL OR stopped = ?)", false)
		if includeOrigin {
			base = base.Where("(client_id = ? OR origin_client_id = ?)", clientID, clientID)
		} else {
			base = base.Where("client_id = ?", clientID)
		}
		var proxies []*models.ProxyConfig
		if err := base.Find(&proxies).Error; err != nil {
			return err
		}
		refs := make([]proxyPortReservationRef, 0, len(proxies))
		for _, proxy := range proxies {
			if proxy != nil && proxy.ProxyConfigEntity != nil {
				if protocol, port, ok := proxyRemotePortForReservation(proxy); ok {
					refs = append(refs, proxyPortReservationRef{
						nodeID: proxy.ServerID, nodeType: models.NodeTypeServer, protocol: protocol,
						port: port, resourceID: proxyReservationIDForProxy(proxy),
					})
				}
			}
		}
		if includeOrigin {
			base = tx.Where("user_id = ? AND tenant_id = ?", userInfo.GetUserID(), userInfo.GetTenantID()).
				Where("(stopped IS NULL OR stopped = ?)", false).
				Where("(client_id = ? OR origin_client_id = ?)", clientID, clientID)
		} else {
			base = tx.Where("user_id = ? AND tenant_id = ?", userInfo.GetUserID(), userInfo.GetTenantID()).
				Where("(stopped IS NULL OR stopped = ?)", false).
				Where("client_id = ?", clientID)
		}
		if err := base.Unscoped().Delete(&models.ProxyConfig{}).Error; err != nil {
			return err
		}
		return releaseProxyReservationRefsIfUnusedTx(tx, userInfo, refs)
	})
}

func (q *proxyQuery) GetProxyConfigByOriginClientIDAndName(userInfo models.UserInfo, clientID string, name string) (*models.ProxyConfig, error) {
	if clientID == "" || name == "" {
		return nil, fmt.Errorf("invalid client id or name")
	}
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	item := &models.ProxyConfig{}
	err := db.
		Where(&models.ProxyConfig{ProxyConfigEntity: &models.ProxyConfigEntity{
			UserID:         userInfo.GetUserID(),
			TenantID:       userInfo.GetTenantID(),
			OriginClientID: clientID,
			Name:           name,
		}}).
		First(&item).Error
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (q *proxyQuery) CountProxyConfigs(userInfo models.UserInfo) (int64, error) {
	return q.CountProxyConfigsWithFilters(userInfo, &models.ProxyConfigEntity{})
}

func (q *proxyQuery) CountProxyConfigsWithFilters(userInfo models.UserInfo, filters *models.ProxyConfigEntity) (int64, error) {
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	filters.UserID = userInfo.GetUserID()
	filters.TenantID = userInfo.GetTenantID()

	var count int64
	err := db.Model(&models.ProxyConfig{}).Where(&models.ProxyConfig{
		ProxyConfigEntity: filters,
	}).Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (q *proxyQuery) CountProxyConfigsWithFiltersAndKeyword(userInfo models.UserInfo, filters *models.ProxyConfigEntity, keyword string) (int64, error) {
	if len(keyword) == 0 {
		return q.CountProxyConfigsWithFilters(userInfo, filters)
	}

	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	filters.UserID = userInfo.GetUserID()
	filters.TenantID = userInfo.GetTenantID()

	var count int64
	err := db.Model(&models.ProxyConfig{}).Where(&models.ProxyConfig{
		ProxyConfigEntity: filters,
	}).Where("name like ?", "%"+keyword+"%").Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (q *proxyQuery) GetProxyConfigsByWorkerId(userInfo models.UserInfo, workerID string) ([]*models.ProxyConfig, error) {
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	items := []*models.ProxyConfig{}

	err := db.
		Where(&models.ProxyConfig{ProxyConfigEntity: &models.ProxyConfigEntity{
			UserID:   userInfo.GetUserID(),
			TenantID: userInfo.GetTenantID(),
		},
			WorkerID: workerID,
		}).
		Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}
