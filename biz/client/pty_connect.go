package client

import (
	"github.com/yunj-yun/frp-web-panel/biz/common"
	"github.com/yunj-yun/frp-web-panel/pb"
	"github.com/yunj-yun/frp-web-panel/services/app"
)

func StartPTYConnect(c *app.Context, req *pb.CommonRequest) (*pb.CommonResponse, error) {
	return common.StartPTYConnect(c, req, &pb.PTYClientMessage{Base: &pb.PTYClientMessage_ClientBase{
		ClientBase: &pb.ClientBase{
			ClientId:     c.GetApp().GetConfig().Client.ID,
			ClientSecret: c.GetApp().GetConfig().Client.Secret,
		},
	}})
}
