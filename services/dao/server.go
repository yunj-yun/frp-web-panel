package dao

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/yunj-yun/frp-web-panel/defs"
	"github.com/yunj-yun/frp-web-panel/models"
	"gorm.io/gorm"
)

type ServerQuery interface {
	GetDefaultServer() (*models.ServerEntity, error)
	ValidateServerSecret(serverID string, secret string) (*models.ServerEntity, error)
	AdminGetServerByServerID(serverID string) (*models.ServerEntity, error)
	GetServerByServerID(userInfo models.UserInfo, serverID string) (*models.ServerEntity, error)
	ListServers(userInfo models.UserInfo, page, pageSize int) ([]*models.ServerEntity, error)
	ListServersWithKeyword(userInfo models.UserInfo, page, pageSize int, keyword string) ([]*models.ServerEntity, error)
	CountServers(userInfo models.UserInfo) (int64, error)
	CountServersWithKeyword(userInfo models.UserInfo, keyword string) (int64, error)
	CountConfiguredServers(userInfo models.UserInfo) (int64, error)
}

type ServerMutation interface {
	InitDefaultServer(serverIP string) error
	UpdateDefaultServer(c *models.Server) error
	CreateServer(userInfo models.UserInfo, server *models.ServerEntity) error
	DeleteServer(userInfo models.UserInfo, serverID string) error
	UpdateServer(userInfo models.UserInfo, server *models.ServerEntity) error
}

type serverQuery struct{ *queryImpl }
type serverMutation struct{ *mutationImpl }

func newServerQuery(base *queryImpl) ServerQuery          { return &serverQuery{base} }
func newServerMutation(base *mutationImpl) ServerMutation { return &serverMutation{base} }

func (m *serverMutation) InitDefaultServer(serverIP string) error {
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	return db.Transaction(func(tx *gorm.DB) error {
		server := &models.Server{ServerEntity: &models.ServerEntity{}}
		if err := tx.Where(&models.Server{
			ServerEntity: &models.ServerEntity{ServerID: defs.DefaultServerID},
		}).Attrs(&models.Server{
			ServerEntity: &models.ServerEntity{
				ServerID:      defs.DefaultServerID,
				ServerIP:      serverIP,
				ConnectSecret: uuid.New().String(),
			},
		}).FirstOrCreate(server).Error; err != nil {
			return err
		}
		config, err := server.GetConfigContent()
		if err != nil && len(server.ConfigContent) > 0 {
			return fmt.Errorf("read default server configuration: %w", err)
		}
		if err != nil {
			config = nil
		}
		userInfo := &models.UserEntity{UserID: server.UserID, TenantID: server.TenantID, Status: models.STATUS_NORMAL}
		return reconcileServerPortReservationsTx(tx, userInfo, defs.DefaultServerID, config)
	})
}

func (q *serverQuery) GetDefaultServer() (*models.ServerEntity, error) {
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	c := &models.Server{}
	err := db.
		Where(&models.Server{ServerEntity: &models.ServerEntity{
			ServerID: defs.DefaultServerID,
		}}).
		First(c).Error
	if err != nil {
		return nil, err
	}
	return c.ServerEntity, nil
}

func (m *serverMutation) UpdateDefaultServer(c *models.Server) error {
	if c == nil || c.ServerEntity == nil {
		return fmt.Errorf("default server is required")
	}
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	c.ServerID = defs.DefaultServerID
	config, err := c.GetConfigContent()
	if err != nil && len(c.ConfigContent) > 0 {
		return fmt.Errorf("read default server configuration: %w", err)
	}
	if err != nil {
		config = nil
	}
	userInfo := &models.UserEntity{UserID: c.UserID, TenantID: c.TenantID, Status: models.STATUS_NORMAL}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where(&models.Server{
			ServerEntity: &models.ServerEntity{ServerID: defs.DefaultServerID},
		}).Save(c).Error; err != nil {
			return err
		}
		return reconcileServerPortReservationsTx(tx, userInfo, defs.DefaultServerID, config)
	})
}

func (q *serverQuery) ValidateServerSecret(serverID string, secret string) (*models.ServerEntity, error) {
	if serverID == "" || secret == "" {
		return nil, fmt.Errorf("invalid request")
	}
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	c := &models.Server{}
	err := db.
		Where(&models.Server{ServerEntity: &models.ServerEntity{
			ServerID: serverID,
		}}).
		First(c).Error
	if err != nil {
		return nil, err
	}
	if c.ConnectSecret != secret {
		return nil, fmt.Errorf("invalid secret")
	}
	return c.ServerEntity, nil
}

func (q *serverQuery) AdminGetServerByServerID(serverID string) (*models.ServerEntity, error) {
	if serverID == "" {
		return nil, fmt.Errorf("invalid server id")
	}
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	c := &models.Server{}
	err := db.
		Where(&models.Server{ServerEntity: &models.ServerEntity{
			ServerID: serverID,
		}}).
		First(c).Error
	if err != nil {
		return nil, err
	}
	return c.ServerEntity, nil
}

func (q *serverQuery) GetServerByServerID(userInfo models.UserInfo, serverID string) (*models.ServerEntity, error) {
	if serverID == "" {
		return nil, fmt.Errorf("invalid server id")
	}
	if userInfo.GetUserID() == defs.DefaultAdminUserID && serverID == defs.DefaultServerID {
		return q.GetDefaultServer()
	}
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	c := &models.Server{}
	err := db.
		Where(&models.Server{ServerEntity: &models.ServerEntity{
			TenantID: userInfo.GetTenantID(),
			UserID:   userInfo.GetUserID(),
			ServerID: serverID,
		}}).
		First(c).Error
	if err != nil {
		return nil, err
	}
	return c.ServerEntity, nil
}

func (m *serverMutation) CreateServer(userInfo models.UserInfo, server *models.ServerEntity) error {
	if userInfo == nil || !userInfo.Valid() || server == nil {
		return fmt.Errorf("user info and server are required")
	}
	server.UserID = userInfo.GetUserID()
	server.TenantID = userInfo.GetTenantID()
	// Persist a default frps configuration alongside the node record. A Server
	// without config content cannot be used to build a client config, because
	// the client needs bindPort to reach frps, so tunnels created before the
	// user visits the Server page used to fail. Seeding the defaults makes a
	// fresh Server immediately usable; saving the Server page overwrites them.
	if len(server.ConfigContent) == 0 {
		if err := server.SetConfigContent(models.DefaultServerConfig()); err != nil {
			return fmt.Errorf("seed default server configuration: %w", err)
		}
	}
	seedConfig, _ := server.GetConfigContent()
	c := &models.Server{
		ServerEntity: server,
	}
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(c).Error; err != nil {
			return err
		}
		return reconcileServerPortReservationsTx(tx, userInfo, server.ServerID, seedConfig)
	})
}

func (m *serverMutation) DeleteServer(userInfo models.UserInfo, serverID string) error {
	if serverID == "" {
		return fmt.Errorf("invalid server id")
	}
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	return db.Transaction(func(tx *gorm.DB) error {
		if err := releaseActivePortReservations(tx.Where("user_id = ? AND tenant_id = ? AND node_id = ? AND node_type = ? AND status = ?",
			userInfo.GetUserID(), userInfo.GetTenantID(), serverID, models.NodeTypeServer, models.PortReservationStatusActive)); err != nil {
			return err
		}
		return tx.Unscoped().Where(
			&models.Server{
				ServerEntity: &models.ServerEntity{
					TenantID: userInfo.GetTenantID(),
					UserID:   userInfo.GetUserID(),
				},
			},
		).Delete(&models.Server{
			ServerEntity: &models.ServerEntity{
				ServerID: serverID,
			},
		}).Error
	})
}

func (m *serverMutation) UpdateServer(userInfo models.UserInfo, server *models.ServerEntity) error {
	if userInfo == nil || !userInfo.Valid() || server == nil {
		return fmt.Errorf("user info and server are required")
	}
	config, err := server.GetConfigContent()
	if err != nil {
		return fmt.Errorf("read server configuration: %w", err)
	}
	c := &models.Server{
		ServerEntity: server,
	}
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	return db.Transaction(func(tx *gorm.DB) error {
		var err error
		if userInfo.GetUserID() == defs.DefaultAdminUserID && server.ServerID == defs.DefaultServerID {
			c.ServerID = defs.DefaultServerID
			err = tx.Where(&models.Server{ServerEntity: &models.ServerEntity{ServerID: defs.DefaultServerID}}).Save(c).Error
		} else {
			err = tx.Where(&models.Server{
				ServerEntity: &models.ServerEntity{UserID: userInfo.GetUserID(), TenantID: userInfo.GetTenantID()},
			}).Save(c).Error
		}
		if err != nil {
			return err
		}
		return reconcileServerPortReservationsTx(tx, userInfo, server.ServerID, config)
	})
}

func (q *serverQuery) ListServers(userInfo models.UserInfo, page, pageSize int) ([]*models.ServerEntity, error) {
	if page < 1 || pageSize < 1 {
		return nil, fmt.Errorf("invalid page or page size")
	}

	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	offset := (page - 1) * pageSize

	var servers []*models.Server
	err := db.Where(
		&models.Server{
			ServerEntity: &models.ServerEntity{
				UserID:   userInfo.GetUserID(),
				TenantID: userInfo.GetTenantID(),
			},
		},
	).Or(&models.Server{
		ServerEntity: &models.ServerEntity{
			ServerID: defs.DefaultServerID,
		},
	}).Offset(offset).Limit(pageSize).Find(&servers).Error
	if err != nil {
		return nil, err
	}

	return lo.Map(servers, func(c *models.Server, _ int) *models.ServerEntity {
		return c.ServerEntity
	}), nil
}

func (q *serverQuery) ListServersWithKeyword(userInfo models.UserInfo, page, pageSize int, keyword string) ([]*models.ServerEntity, error) {
	if page < 1 || pageSize < 1 || len(keyword) == 0 {
		return nil, fmt.Errorf("invalid page or page size or keyword")
	}

	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	offset := (page - 1) * pageSize

	var servers []*models.Server
	err := db.Where(
		&models.Server{
			ServerEntity: &models.ServerEntity{
				UserID:   userInfo.GetUserID(),
				TenantID: userInfo.GetTenantID(),
			},
		},
	).Where("server_id like ?", "%"+keyword+"%").
		Offset(offset).Limit(pageSize).Find(&servers).Error
	if err != nil {
		return nil, err
	}

	return lo.Map(servers, func(c *models.Server, _ int) *models.ServerEntity {
		return c.ServerEntity
	}), nil
}

func (q *serverQuery) CountServers(userInfo models.UserInfo) (int64, error) {
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	var count int64
	err := db.Model(&models.Server{}).Where(
		&models.Server{
			ServerEntity: &models.ServerEntity{
				UserID:   userInfo.GetUserID(),
				TenantID: userInfo.GetTenantID(),
			},
		},
	).Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (q *serverQuery) CountServersWithKeyword(userInfo models.UserInfo, keyword string) (int64, error) {
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	var count int64
	err := db.Model(&models.Server{}).Where(
		&models.Server{
			ServerEntity: &models.ServerEntity{
				UserID:   userInfo.GetUserID(),
				TenantID: userInfo.GetTenantID(),
			},
		},
	).Where("server_id like ?", "%"+keyword+"%").Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (q *serverQuery) CountConfiguredServers(userInfo models.UserInfo) (int64, error) {
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	var count int64
	err := db.Model(&models.Server{}).Where(
		&models.Server{
			ServerEntity: &models.ServerEntity{
				UserID:   userInfo.GetUserID(),
				TenantID: userInfo.GetTenantID(),
			},
		},
	).Not(
		&models.Server{
			ServerEntity: &models.ServerEntity{
				ConfigContent: []byte{},
			},
		},
	).Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}
