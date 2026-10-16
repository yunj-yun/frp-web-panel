package proxy

import (
	"context"
	"time"

	"github.com/yunj-yun/frp-web-panel/models"
	"github.com/yunj-yun/frp-web-panel/services/app"
	"github.com/yunj-yun/frp-web-panel/services/dao"
	"github.com/yunj-yun/frp-web-panel/utils/logger"
	"github.com/samber/lo"
)

func CollectDailyStats(appInstance app.Application) error {
	ctx := app.NewContext(context.Background(), appInstance)

	tx := appInstance.GetDBManager().GetDefaultDB().Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	proxies, err := dao.NewQuery(ctx).AdminGetAllProxyStats(tx)
	if err != nil {
		logger.Logger(context.Background()).WithError(err).Error("CollectDailyStats cannot get proxies")
		return err
	}

	proxyDailyStats := lo.Map(proxies, func(item *models.ProxyStatsEntity, _ int) *models.HistoryProxyStats {
		now := time.Now()
		statDate := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, now.Location())
		return &models.HistoryProxyStats{
			ProxyID:        item.ProxyID,
			ServerID:       item.ServerID,
			ClientID:       item.ClientID,
			OriginClientID: item.OriginClientID,
			Name:           item.Name,
			Type:           item.Type,
			UserID:         item.UserID,
			TenantID:       item.TenantID,
			TrafficIn:      item.HistoryTrafficIn,
			TrafficOut:     item.HistoryTrafficOut,
			StatDate:       statDate,
		}
	})

	if err := dao.NewMutation(ctx).AdminMSaveTodyStats(tx, proxyDailyStats); err != nil {
		logger.Logger(context.Background()).WithError(err).Error("CollectDailyStats cannot save stats")
		return err
	}

	logger.Logger(ctx).Infof("CollectDailyStats success")

	return nil
}
