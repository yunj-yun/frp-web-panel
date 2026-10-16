package server

import (
	"github.com/yunj-yun/frp-web-panel/models"
	"github.com/yunj-yun/frp-web-panel/pb"
	"github.com/yunj-yun/frp-web-panel/services/app"
	"github.com/samber/lo"
)

func RPCPullConfig(ctx *app.Context, req *pb.PullServerConfigReq) (*pb.PullServerConfigResp, error) {
	var cli *models.ServerEntity
	var err error

	if cli, err = ValidateServerRequest(ctx, req.GetBase()); err != nil {
		return nil, err
	}

	return &pb.PullServerConfigResp{
		Server: &pb.Server{
			Id:     lo.ToPtr(cli.ServerID),
			Config: lo.ToPtr(string(cli.ConfigContent)),
		},
	}, nil
}
