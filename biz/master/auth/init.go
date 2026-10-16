package auth

import (
	"context"

	"github.com/yunj-yun/frp-web-panel/models"
	"github.com/yunj-yun/frp-web-panel/services/app"
	"github.com/yunj-yun/frp-web-panel/services/cache"
	"github.com/yunj-yun/frp-web-panel/services/dao"
	"github.com/yunj-yun/frp-web-panel/utils/logger"
	"github.com/samber/lo"
)

func InitAuth(appInstance app.Application) {
	appCtx := app.NewContext(context.Background(), appInstance)
	logger.Logger(appCtx).Info("start to init frp user auth token")

	u, err := dao.NewQuery(appCtx).AdminGetAllUsers()
	if err != nil {
		logger.Logger(context.Background()).WithError(err).Fatalf("init frp user auth token failed")
	}

	lo.ForEach(u, func(user *models.UserEntity, _ int) {
		cache.Get().Set([]byte(user.GetUserName()), []byte(user.GetToken()), 0)
	})

	logger.Logger(appCtx).Infof("init frp user auth token success, count: %d", len(u))
}
