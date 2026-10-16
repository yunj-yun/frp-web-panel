package proxy

import (
	"fmt"

	"github.com/yunj-yun/frp-web-panel/biz/master/client"
	"github.com/yunj-yun/frp-web-panel/common"
	"github.com/yunj-yun/frp-web-panel/defs"
	"github.com/yunj-yun/frp-web-panel/pb"
	"github.com/yunj-yun/frp-web-panel/services/app"
	"github.com/yunj-yun/frp-web-panel/services/dao"
	"github.com/yunj-yun/frp-web-panel/utils/logger"
	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/samber/lo"
)

func DeleteProxyConfig(c *app.Context, req *pb.DeleteProxyConfigRequest) (*pb.DeleteProxyConfigResponse, error) {
	var (
		userInfo  = common.GetUserInfo(c)
		clientID  = req.GetClientId()
		serverID  = req.GetServerId()
		proxyName = req.GetName()
	)
	q := dao.NewQuery(c)
	m := dao.NewMutation(c)

	if len(clientID) == 0 || len(serverID) == 0 || len(proxyName) == 0 {
		return nil, fmt.Errorf("request invalid")
	}

	cli, err := q.GetClientByClientID(userInfo, clientID)
	if err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot get client, id: [%s]", clientID)
		return nil, err
	}
	if cli.ServerID != serverID {
		return nil, fmt.Errorf("client and server not match")
	}

	oldCfg, err := cli.GetConfigContent()
	if err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot get client config content, id: [%s]", clientID)
		return nil, err
	}
	var deletedProxy v1.TypedProxyConfig
	for _, configured := range oldCfg.Proxies {
		if configured.GetBaseConfig() != nil && configured.GetBaseConfig().Name == proxyName {
			deletedProxy = configured
			break
		}
	}

	oldCfg.Proxies = lo.Filter(oldCfg.Proxies, func(p v1.TypedProxyConfig, _ int) bool {
		return p.GetBaseConfig().Name != proxyName
	})

	if err := cli.SetConfigContent(*oldCfg); err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot set client config, id: [%s]", clientID)
		return nil, err
	}

	if err := m.UpdateClient(userInfo, cli.ClientEntity); err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot update client, id: [%s]", clientID)
		return nil, err
	}

	rawCfg, err := cli.MarshalJSONConfig()
	if err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot marshal client config, id: [%s]", clientID)
		return nil, err
	}

	_, err = client.UpdateFrpcHander(c, &pb.UpdateFRPCRequest{
		ClientId: &cli.ClientID,
		ServerId: &serverID,
		Config:   rawCfg,
		Comment:  &cli.Comment,
		FrpsUrl:  &cli.FrpsUrl,
	})
	if err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot update frpc, id: [%s]", clientID)
		return nil, err
	}

	if err := m.DeleteProxyConfig(userInfo, clientID, proxyName); err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot delete proxy config, id: [%s]", clientID)
		return nil, err
	}
	// A deleted tunnel no longer owns its server-side TCP/UDP port. Release the
	// reservation after the configuration record is removed; historical rows
	// remain available for audit without blocking a later reuse.
	if protocol, port, ok := proxyRemotePortValue(deletedProxy); ok {
		resourceID := proxyReservationResourceID(serverID, clientID, cli.OriginClientID, proxyName)
		if releaseErr := m.ReleaseProxyPortIfUnused(userInfo, serverID, defs.CliTypeServer, protocol, port, resourceID); releaseErr != nil {
			logger.Logger(c).WithError(releaseErr).Warnf("cannot release proxy port reservation, id: [%s], name: [%s]", clientID, proxyName)
		}
	}

	logger.Logger(c).Infof("delete proxy config, id: [%s], name: [%s]", clientID, proxyName)

	return &pb.DeleteProxyConfigResponse{}, nil
}
