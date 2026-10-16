package client

import (
	"context"
	"runtime"
	"time"

	"github.com/VaalaCat/frp-panel/biz/common"
	"github.com/VaalaCat/frp-panel/pb"
	"github.com/VaalaCat/frp-panel/services/app"
	"github.com/VaalaCat/frp-panel/services/ports"
	"github.com/VaalaCat/frp-panel/utils/logger"
)

func ReportNodePortSnapshot(appInstance app.Application, clientID, clientSecret string) error {
	if runtime.GOOS != "linux" || appInstance.GetMasterCli() == nil {
		return nil
	}

	sockets, err := ports.NewScanner().Scan(context.Background())
	if err != nil {
		return err
	}
	_, err = appInstance.GetMasterCli().Call().ReportNodePortSnapshot(context.Background(), &pb.ReportNodePortSnapshotReq{
		ClientBase: &pb.ClientBase{ClientId: clientID, ClientSecret: clientSecret},
		ObservedAt: time.Now().UnixMilli(),
		Sockets:    common.PortSockets(sockets),
	})
	if err != nil {
		logger.Logger(context.Background()).WithError(err).Warnf("report client port snapshot failed, client id: [%s]", clientID)
	}
	return err
}
