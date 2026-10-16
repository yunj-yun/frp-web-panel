package server

import (
	"context"
	"reflect"

	"github.com/yunj-yun/frp-web-panel/pb"
	"github.com/yunj-yun/frp-web-panel/services/app"
	"github.com/yunj-yun/frp-web-panel/services/server"
	"github.com/yunj-yun/frp-web-panel/utils"
	"github.com/yunj-yun/frp-web-panel/utils/logger"
)

func UpdateFrpsHander(ctx *app.Context, req *pb.UpdateFRPSRequest) (*pb.UpdateFRPSResponse, error) {
	logger.Logger(ctx).Infof("update frps, serverID: [%s], configBytes: [%d]", req.GetServerId(), len(req.GetConfig()))

	content := req.GetConfig()

	s, err := utils.LoadServerConfig(content, true)
	if err != nil {
		logger.Logger(context.Background()).WithError(err).Errorf("cannot load config")
		return &pb.UpdateFRPSResponse{
			Status: &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: err.Error()},
		}, err
	}

	InjectAuthPlugin(ctx, s)

	serverID := req.GetServerId()
	if cli := ctx.GetApp().GetServerController().Get(serverID); cli != nil {
		if !reflect.DeepEqual(cli.GetCommonCfg(), s) {
			cli.Stop()
			ctx.GetApp().GetServerController().Delete(serverID)
			logger.Logger(ctx).Infof("server %s config changed, will recreate it", serverID)
		} else {
			logger.Logger(ctx).Infof("server %s config not changed", serverID)
			return &pb.UpdateFRPSResponse{
				Status: &pb.Status{Code: pb.RespCode_RESP_CODE_SUCCESS, Message: "ok"},
			}, nil
		}
	}

	ctx.GetApp().GetServerController().Add(serverID, server.NewServerHandler(s))
	ctx.GetApp().GetServerController().Run(serverID)

	return &pb.UpdateFRPSResponse{
		Status: &pb.Status{Code: pb.RespCode_RESP_CODE_SUCCESS, Message: "ok"},
	}, nil
}
