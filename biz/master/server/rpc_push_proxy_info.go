package server

import (
	"github.com/yunj-yun/frp-web-panel/models"
	"github.com/yunj-yun/frp-web-panel/pb"
	"github.com/yunj-yun/frp-web-panel/services/app"
	"github.com/yunj-yun/frp-web-panel/services/dao"
)

func PushProxyInfo(ctx *app.Context, req *pb.PushProxyInfoReq) (*pb.PushProxyInfoResp, error) {
	var srv *models.ServerEntity
	var err error

	if srv, err = ValidateServerRequest(ctx, req.GetBase()); err != nil {
		return nil, err
	}

	if err = dao.NewMutation(ctx).AdminUpdateProxyStats(srv, req.GetProxyInfos()); err != nil {
		return nil, err
	}
	return &pb.PushProxyInfoResp{
		Status: &pb.Status{Code: pb.RespCode_RESP_CODE_SUCCESS, Message: "ok"},
	}, nil
}
