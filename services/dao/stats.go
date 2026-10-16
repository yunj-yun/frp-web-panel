package dao

import (
	"fmt"
	"time"

	"github.com/VaalaCat/frp-panel/models"
	"gorm.io/gorm"
)

const (
	MSetBatchSize = 100
)

type StatsQuery interface {
	GetHistoryStatsByProxyID(userInfo models.UserInfo, proxyID int) ([]*models.HistoryProxyStats, error)
	GetHistoryStatsByClientID(userInfo models.UserInfo, clientID string) ([]*models.HistoryProxyStats, error)
	GetHistoryStatsByServerID(userInfo models.UserInfo, serverID string) ([]*models.HistoryProxyStats, error)
	ListHistoryStats(userInfo models.UserInfo, clientID, serverID string, proxyID uint) ([]*models.HistoryProxyStats, error)
	ListHistoryStatsForRange(userInfo models.UserInfo, clientID, serverID string, proxyID uint, start, end time.Time) ([]*models.HistoryProxyStats, error)
}

type StatsMutation interface {
	AdminSaveTodyStats(s *models.HistoryProxyStats) error
	AdminMSaveTodyStats(tx *gorm.DB, s []*models.HistoryProxyStats) error
}

type statsQuery struct{ *queryImpl }
type statsMutation struct{ *mutationImpl }

func newStatsQuery(base *queryImpl) StatsQuery          { return &statsQuery{base} }
func newStatsMutation(base *mutationImpl) StatsMutation { return &statsMutation{base} }

func (m *statsMutation) AdminSaveTodyStats(s *models.HistoryProxyStats) error {
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	return db.Save(s).Error
}

func (m *statsMutation) AdminMSaveTodyStats(tx *gorm.DB, s []*models.HistoryProxyStats) error {
	if len(s) == 0 {
		return nil
	}

	if err := tx.CreateInBatches(s, MSetBatchSize).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("AdminMSaveTodyStats failed to save history proxy stats: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("AdminMSaveTodyStats failed to commit transaction: %v", err)
	}
	return nil
}

func (q *statsQuery) GetHistoryStatsByProxyID(userInfo models.UserInfo, proxyID int) ([]*models.HistoryProxyStats, error) {
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	var stats []*models.HistoryProxyStats
	err := db.Where(&models.HistoryProxyStats{
		ProxyID:  proxyID,
		UserID:   userInfo.GetUserID(),
		TenantID: userInfo.GetTenantID(),
	}).Find(&stats).Error
	if err != nil {
		return nil, err
	}
	return stats, nil
}

func (q *statsQuery) GetHistoryStatsByClientID(userInfo models.UserInfo, clientID string) ([]*models.HistoryProxyStats, error) {
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	var stats []*models.HistoryProxyStats
	err := db.Where(&models.HistoryProxyStats{
		ClientID: clientID,
		UserID:   userInfo.GetUserID(),
		TenantID: userInfo.GetTenantID(),
	}).Find(&stats).Error
	if err != nil {
		return nil, err
	}
	return stats, nil
}

func (q *statsQuery) GetHistoryStatsByServerID(userInfo models.UserInfo, serverID string) ([]*models.HistoryProxyStats, error) {
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB()
	var stats []*models.HistoryProxyStats
	err := db.Where(&models.HistoryProxyStats{
		ServerID: serverID,
		UserID:   userInfo.GetUserID(),
		TenantID: userInfo.GetTenantID(),
	}).Find(&stats).Error
	if err != nil {
		return nil, err
	}
	return stats, nil
}

// ListHistoryStats returns all snapshots for a tenant and optional resource
// scope. Date filtering and cumulative-to-daily conversion belong to the API
// layer because the caller supplies the display timezone and range.
func (q *statsQuery) ListHistoryStats(userInfo models.UserInfo, clientID, serverID string, proxyID uint) ([]*models.HistoryProxyStats, error) {
	if userInfo == nil {
		return nil, fmt.Errorf("user info is required")
	}
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB().Where("user_id = ? AND tenant_id = ?", userInfo.GetUserID(), userInfo.GetTenantID())
	if clientID != "" {
		db = db.Where("(client_id = ? OR origin_client_id = ?)", clientID, clientID)
	}
	if serverID != "" {
		db = db.Where("server_id = ?", serverID)
	}
	if proxyID != 0 {
		db = db.Where("proxy_id = ?", proxyID)
	}
	var stats []*models.HistoryProxyStats
	if err := db.Order("proxy_id ASC, stat_date ASC, created_at ASC, id ASC").Limit(20000).Find(&stats).Error; err != nil {
		return nil, err
	}
	return stats, nil
}

// ListHistoryStatsForRange reads a bounded window that includes enough history
// before the requested start to calculate a cumulative-counter baseline. The
// API still performs the exact half-open range filtering after sorting rows.
func (q *statsQuery) ListHistoryStatsForRange(userInfo models.UserInfo, clientID, serverID string, proxyID uint, start, end time.Time) ([]*models.HistoryProxyStats, error) {
	if userInfo == nil || !userInfo.Valid() {
		return nil, fmt.Errorf("user info is required")
	}
	if end.IsZero() || !end.After(start) {
		return nil, fmt.Errorf("invalid history range")
	}
	lower := start.Add(-(366*24 + 24) * time.Hour)
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB().Where("user_id = ? AND tenant_id = ?", userInfo.GetUserID(), userInfo.GetTenantID())
	if clientID != "" {
		db = db.Where("(client_id = ? OR origin_client_id = ?)", clientID, clientID)
	}
	if serverID != "" {
		db = db.Where("server_id = ?", serverID)
	}
	if proxyID != 0 {
		db = db.Where("proxy_id = ?", proxyID)
	}
	// New rows use StatDate. The CreatedAt branch keeps older installations
	// readable where StatDate was stored as a zero value rather than NULL.
	db = db.Where("((stat_date >= ? AND stat_date < ?) OR (created_at >= ? AND created_at < ?))", lower, end, lower, end)
	var stats []*models.HistoryProxyStats
	if err := db.Order("stat_date ASC, created_at ASC, id ASC").Limit(50000).Find(&stats).Error; err != nil {
		return nil, err
	}
	return stats, nil
}
