package worker

import (
	"github.com/yunj-yun/frp-web-panel/common"
	"github.com/yunj-yun/frp-web-panel/models"
	"github.com/yunj-yun/frp-web-panel/pb"
	"github.com/yunj-yun/frp-web-panel/services/app"
	"github.com/yunj-yun/frp-web-panel/services/dao"
	"github.com/yunj-yun/frp-web-panel/utils/logger"
	"github.com/samber/lo"
)

func GetWorkerIngress(ctx *app.Context, req *pb.GetWorkerIngressRequest) (*pb.GetWorkerIngressResponse, error) {
	logger.Logger(ctx).Infof("get worker: [%s] ingress", req.GetWorkerId())
	var (
		workerId = req.GetWorkerId()
		userInfo = common.GetUserInfo(ctx)
	)

	proxyCfgs, err := dao.NewQuery(ctx).GetProxyConfigsByWorkerId(userInfo, workerId)
	if err != nil {
		logger.Logger(ctx).WithError(err).Errorf("failed to get proxy configs for worker: [%s]", workerId)
		return nil, err
	}

	logger.Logger(ctx).Infof("got proxy configs for worker: [%s] success, ingresses length: [%d]", workerId, len(proxyCfgs))
	return &pb.GetWorkerIngressResponse{
		ProxyConfigs: lo.Map(proxyCfgs, func(item *models.ProxyConfig, index int) *pb.ProxyConfig {
			return item.ToPB()
		}),
	}, nil
}
