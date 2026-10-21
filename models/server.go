package models

import (
	"encoding/json"
	"fmt"
	"time"

	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/yunj-yun/frp-web-panel/utils"
	"gorm.io/gorm"
)

type Server struct {
	*ServerEntity
}

type ServerEntity struct {
	ServerID      string            `json:"client_id" gorm:"uniqueIndex;not null;primaryKey"`
	TenantID      int               `json:"tenant_id" gorm:"not null,index"`
	UserID        int               `json:"user_id" gorm:"not null"`
	ServerIP      string            `json:"server_ip"`
	ConfigContent []byte            `json:"config_content"`
	ConnectSecret string            `json:"connect_secret" gorm:"not null"`
	Comment       string            `json:"comment"`
	FrpsUrls      GormArray[string] `json:"frps_urls"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     gorm.DeletedAt `gorm:"index"`
}

func (*Server) TableName() string {
	return "servers"
}

func (s *ServerEntity) GetConfigContent() (*v1.ServerConfig, error) {
	if len(s.ConfigContent) == 0 {
		return nil, fmt.Errorf("config content is empty")
	}

	var cfg v1.ServerConfig
	err := json.Unmarshal(s.ConfigContent, &cfg)
	return &cfg, err
}

func (s *ServerEntity) SetConfigContent(cfg *v1.ServerConfig) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	s.ConfigContent = raw
	return nil
}

func (s *ServerEntity) ConfigEqual(cfg *v1.ServerConfig) bool {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return false
	}

	return utils.MD5(s.ConfigContent) == utils.MD5(raw)
}

// DefaultServerConfig returns the frps defaults used when a Server is created
// or repaired. A Server without configuration cannot build a client config
// because the client needs bindPort to reach frps, so both the create path and
// the read path seed these defaults instead of leaving the record unusable.
// frp's own Complete() supplies bindPort 7000 and the remaining values, keeping
// the document consistent with what the Server page renders.
func DefaultServerConfig() *v1.ServerConfig {
	cfg := &v1.ServerConfig{}
	_ = cfg.Complete()
	return cfg
}
