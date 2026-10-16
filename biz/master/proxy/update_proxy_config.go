package proxy

import (
	"fmt"
	"strings"

	"github.com/yunj-yun/frp-web-panel/biz/master/client"
	"github.com/yunj-yun/frp-web-panel/common"
	"github.com/yunj-yun/frp-web-panel/defs"
	"github.com/yunj-yun/frp-web-panel/models"
	"github.com/yunj-yun/frp-web-panel/pb"
	"github.com/yunj-yun/frp-web-panel/services/app"
	"github.com/yunj-yun/frp-web-panel/services/dao"
	"github.com/yunj-yun/frp-web-panel/utils"
	"github.com/yunj-yun/frp-web-panel/utils/logger"
	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/msg"
	"github.com/samber/lo"
)

func UpdateProxyConfig(c *app.Context, req *pb.UpdateProxyConfigRequest) (*pb.UpdateProxyConfigResponse, error) {
	if len(req.GetClientId()) == 0 || len(req.GetServerId()) == 0 || len(req.GetConfig()) == 0 {
		return nil, fmt.Errorf("request invalid")
	}

	var (
		userInfo = common.GetUserInfo(c)
		clientID = req.GetClientId()
		serverID = req.GetServerId()
	)
	q := dao.NewQuery(c)
	m := dao.NewMutation(c)

	cli, err := q.GetClientByClientID(userInfo, clientID)
	if err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot get client, id: [%s]", clientID)
		return nil, err
	}

	clientEntity := cli.ClientEntity

	if clientEntity.ServerID != serverID {
		logger.Logger(c).Errorf("client and server not match, find or create client, client: [%s], server: [%s]", clientID, serverID)
		originClient, err := q.GetClientByClientID(userInfo, clientEntity.OriginClientID)
		if err != nil {
			logger.Logger(c).WithError(err).Errorf("cannot get origin client, id: [%s]", clientEntity.OriginClientID)
			return nil, err
		}

		clientEntity, _, err = client.ChildClientForServer(c, serverID, originClient.ClientEntity)
		if err != nil {
			logger.Logger(c).WithError(err).Errorf("cannot create child client, id: [%s]", clientID)
			return nil, err
		}
	}

	_, err = q.GetServerByServerID(userInfo, serverID)
	if err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot get server, id: [%s]", serverID)
		return nil, err
	}

	proxyCfg := &models.ProxyConfigEntity{}
	if err := proxyCfg.FillClientConfig(clientEntity); err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot fill client config, id: [%s]", clientID)
		return nil, err
	}

	typedProxyCfgs, err := utils.LoadProxiesFromContent(req.GetConfig())
	if err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot load proxies from content")
		return nil, err
	}
	if len(typedProxyCfgs) == 0 || len(typedProxyCfgs) > 1 {
		logger.Logger(c).Errorf("invalid config, cfg len: [%d]", len(typedProxyCfgs))
		return nil, fmt.Errorf("invalid config")
	}

	typedProxyCfg := typedProxyCfgs[0]

	if typedProxyCfg.GetBaseConfig().Type == string(v1.ProxyTypeHTTP) {
		typedProxyCfg = UpdateWorkerLoadBalancerGroup(typedProxyCfg)
	}

	if err := proxyCfg.FillTypedProxyConfig(typedProxyCfg); err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot fill typed proxy config")
		return nil, err
	}

	oldProxyName := strings.TrimSpace(req.GetName())
	if oldProxyName == "" {
		oldProxyName = proxyCfg.Name
	}
	// Proxy records are owned by the origin client even when the request comes
	// from the server-specific shadow client shown in the tunnel list.
	originClientID := clientEntity.OriginClientID
	if originClientID == "" {
		originClientID = clientEntity.ClientID
	}
	oldProxyCfg, err := q.GetProxyConfigByOriginClientIDAndName(userInfo, originClientID, oldProxyName)
	if err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot get proxy config, id: [%s]", clientID)
		return nil, err
	}
	oldTypedProxyCfg, oldTypedErr := oldProxyCfg.GetTypedProxyConfig()
	if oldTypedErr != nil {
		logger.Logger(c).WithError(oldTypedErr).Errorf("cannot load existing proxy config, id: [%s], name: [%s]", clientID, oldProxyName)
		return nil, oldTypedErr
	}

	reservationCreated, err := validateRemotePortAvailable(c, userInfo, serverID, clientID, clientEntity.OriginClientID, typedProxyCfg, proxyRecordID(oldProxyCfg))
	if err != nil {
		logger.Logger(c).WithError(err).Errorf("proxy remote port is unavailable, client: [%s], server: [%s], proxy: [%s]", clientID, serverID, proxyCfg.Name)
		return nil, err
	}
	reservationProtocol, reservationPort, reservationHasPort := proxyRemotePortValue(typedProxyCfg)
	reservationCommitted := false
	newResourceID := proxyReservationResourceID(serverID, clientEntity.ClientID, clientEntity.OriginClientID, proxyCfg.Name)
	if reservationCreated && reservationHasPort {
		defer func() {
			if reservationCommitted {
				return
			}
			if releaseErr := m.ReleasePort(userInfo, serverID, defs.CliTypeServer, reservationProtocol, reservationPort, models.PortReservationTypeProxy, newResourceID); releaseErr != nil {
				logger.Logger(c).WithError(releaseErr).Warnf("cannot roll back proxy port reservation, client: [%s], proxy: [%s]", clientID, proxyCfg.Name)
			}
		}()
	}

	if m.UpdateProxyConfig(userInfo, &models.ProxyConfig{
		Model:             oldProxyCfg.Model,
		ProxyConfigEntity: proxyCfg,
	}) != nil {
		logger.Logger(c).Errorf("update proxy config failed, client: [%s], server: [%s], proxy: [%s]", clientID, serverID, proxyCfg.Name)
		return nil, fmt.Errorf("update proxy config failed")
	}
	// A successful edit may move or rename a TCP/UDP tunnel. The new
	// reservation was acquired during validation; release the previous intent
	// only after the proxy row has been updated.
	// update client config
	if oldCfg, err := clientEntity.GetConfigContent(); err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot get client config, id: [%s]", clientID)
		return nil, err
	} else {
		oldCfg.Proxies = lo.Filter(oldCfg.Proxies, func(proxy v1.TypedProxyConfig, _ int) bool {
			return proxy.GetBaseConfig().Name != oldProxyName
		})
		oldCfg.Proxies = append(oldCfg.Proxies, typedProxyCfg)

		if err := clientEntity.SetConfigContent(*oldCfg); err != nil {
			logger.Logger(c).WithError(err).Errorf("cannot set client config, id: [%s]", clientID)
			return nil, err
		}
	}

	rawCfg, err := clientEntity.MarshalJSONConfig()
	if err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot marshal client config, id: [%s]", clientID)
		return nil, err
	}

	_, err = client.UpdateFrpcHander(c, &pb.UpdateFRPCRequest{
		ClientId: &clientEntity.ClientID,
		ServerId: &serverID,
		Config:   rawCfg,
		Comment:  &clientEntity.Comment,
		FrpsUrl:  &clientEntity.FrpsUrl,
	})
	if err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot update frpc, id: [%s]", clientID)
		return nil, err
	}
	oldProtocol, oldPort, oldHasPort := proxyRemotePortValue(oldTypedProxyCfg)
	newProtocol, newPort, newHasPort := proxyRemotePortValue(typedProxyCfg)
	oldResourceID := proxyReservationResourceID(oldProxyCfg.ServerID, oldProxyCfg.ClientID, oldProxyCfg.OriginClientID, oldProxyName)
	if oldHasPort && (!newHasPort || oldProtocol != newProtocol || oldPort != newPort || oldResourceID != newResourceID) {
		if releaseErr := m.ReleaseProxyPortIfUnused(userInfo, oldProxyCfg.ServerID, defs.CliTypeServer, oldProtocol, oldPort, oldResourceID); releaseErr != nil {
			logger.Logger(c).WithError(releaseErr).Warnf("cannot release old proxy port reservation, id: [%s], name: [%s]", clientID, oldProxyName)
		}
	}
	reservationCommitted = true

	return &pb.UpdateProxyConfigResponse{
		Status: &pb.Status{Code: pb.RespCode_RESP_CODE_SUCCESS, Message: "ok"},
	}, nil
}

func UpdateWorkerLoadBalancerGroup(typedProxyCfg v1.TypedProxyConfig) v1.TypedProxyConfig {
	annotations := typedProxyCfg.GetBaseConfig().Annotations
	workerId := ""
	if len(annotations) > 0 {
		if annotations[defs.FrpProxyAnnotationsKey_Ingress] != "" && len(annotations[defs.FrpProxyAnnotationsKey_WorkerId]) > 0 {
			workerId = annotations[defs.FrpProxyAnnotationsKey_WorkerId]
		}
	}
	if workerId == "" {
		return typedProxyCfg
	}
	httpProxyCfg := &v1.HTTPProxyConfig{}
	msg := &msg.NewProxy{}
	typedProxyCfg.ProxyConfigurer.MarshalToMsg(msg)
	httpProxyCfg.UnmarshalFromMsg(msg)

	httpProxyCfg.LoadBalancer = v1.LoadBalancerConfig{
		Group:    models.HttpIngressLBGroup(workerId, httpProxyCfg),
		GroupKey: workerId,
	}

	typedProxyCfg.ProxyConfigurer = httpProxyCfg

	return typedProxyCfg
}
