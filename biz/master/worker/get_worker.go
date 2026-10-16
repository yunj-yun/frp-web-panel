package worker

import (
	"fmt"

	"github.com/yunj-yun/frp-web-panel/common"
	"github.com/yunj-yun/frp-web-panel/models"
	"github.com/yunj-yun/frp-web-panel/pb"
	"github.com/yunj-yun/frp-web-panel/services/app"
	"github.com/yunj-yun/frp-web-panel/services/dao"
	"github.com/yunj-yun/frp-web-panel/utils/logger"
	"github.com/samber/lo"
)

func GetWorker(ctx *app.Context, req *pb.GetWorkerRequest) (*pb.GetWorkerResponse, error) {
	logger.Logger(ctx).Infof("get worker req: %s", req.String())
	var (
		workerID = req.GetWorkerId()
		userInfo = common.GetUserInfo(ctx)
	)

	if len(workerID) == 0 {
		logger.Logger(ctx).Errorf("worker id is empty")
		return nil, fmt.Errorf("worker id is empty")
	}

	workerRecord, err := dao.NewQuery(ctx).GetWorkerByWorkerID(userInfo, workerID)
	if err != nil {
		logger.Logger(ctx).WithError(err).Errorf("get worker by id failed")
		return nil, err
	}

	return &pb.GetWorkerResponse{
		Status: &pb.Status{
			Code:    pb.RespCode_RESP_CODE_SUCCESS,
			Message: "ok",
		},
		Worker: workerRecord.ToPB(),
		Clients: lo.Map(workerRecord.Clients, func(client models.Client, index int) *pb.Client {
			c := client.ToPB()
			c.Config = nil
			c.Secret = nil
			return c
		}),
	}, nil
}
