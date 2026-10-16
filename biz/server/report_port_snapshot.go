package server

import (
	"context"
	"runtime"
	"time"

	"github.com/yunj-yun/frp-web-panel/biz/common"
	"github.com/yunj-yun/frp-web-panel/pb"
	"github.com/yunj-yun/frp-web-panel/services/app"
	"github.com/yunj-yun/frp-web-panel/services/ports"
	"github.com/yunj-yun/frp-web-panel/utils/logger"
)

func ReportNodePortSnapshot(appInstance app.Application, serverID, serverSecret string) error {
	if runtime.GOOS != "linux" || appInstance.GetMasterCli() == nil {
		return nil
	}

	sockets, err := ports.NewScanner().Scan(context.Background())
	if err != nil {
		return err
	}
	_, err = appInstance.GetMasterCli().Call().ReportNodePortSnapshot(context.Background(), &pb.ReportNodePortSnapshotReq{
		ServerBase: &pb.ServerBase{ServerId: serverID, ServerSecret: serverSecret},
		ObservedAt: time.Now().UnixMilli(),
		Sockets:    common.PortSockets(sockets),
	})
	if err != nil {
		logger.Logger(context.Background()).WithError(err).Warnf("report server port snapshot failed, server id: [%s]", serverID)
	}
	return err
}
