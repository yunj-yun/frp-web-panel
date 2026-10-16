package proxy

import (
	"errors"
	"fmt"

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
	"github.com/samber/lo"

	"gorm.io/gorm"
)

func CreateProxyConfig(c *app.Context, req *pb.CreateProxyConfigRequest) (*pb.CreateProxyConfigResponse, error) {

	if len(req.GetClientId()) == 0 || len(req.GetServerId()) == 0 || len(req.GetConfig()) == 0 {
		return nil, fmt.Errorf("request invalid")
	}

	var (
		userInfo = common.GetUserInfo(c)
		clientID = req.GetClientId()
		serverID = req.GetServerId()
	)

	clientEntity, err := GetClientWithMakeShadow(c, clientID, serverID)
	if err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot get client, id: [%s]", clientID)
		return nil, err
	}

	_, err = dao.NewQuery(c).GetServerByServerID(userInfo, serverID)
	if err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot get server, id: [%s]", serverID)
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

	if err := CreateProxyConfigWithTypedConfig(c, CreateProxyConfigWithTypedConfigParam{
		ClientID:     clientID,
		ServerID:     serverID,
		ProxyCfg:     typedProxyCfgs[0],
		ClientEntity: clientEntity,
		Overwrite:    req.GetOverwrite(),
	}); err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot create proxy config")
		return nil, err
	}

	return &pb.CreateProxyConfigResponse{
		Status: &pb.Status{Code: pb.RespCode_RESP_CODE_SUCCESS, Message: "ok"},
	}, nil
}

type CreateProxyConfigWithTypedConfigParam struct {
	ClientID     string
	ServerID     string
	ProxyCfg     v1.TypedProxyConfig
	ClientEntity *models.ClientEntity
	Overwrite    bool
	WorkerID     *string
}

func CreateProxyConfigWithTypedConfig(c *app.Context, param CreateProxyConfigWithTypedConfigParam) error {
	var (
		userInfo      = common.GetUserInfo(c)
		clientID      = param.ClientID
		serverID      = param.ServerID
		clientEntity  = param.ClientEntity
		typedProxyCfg = param.ProxyCfg
		err           error
		overwrite     = param.Overwrite
	)

	proxyCfg := &models.ProxyConfigEntity{}

	if err := proxyCfg.FillClientConfig(clientEntity); err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot fill client config, id: [%s]", clientID)
		return err
	}

	if err := proxyCfg.FillTypedProxyConfig(typedProxyCfg); err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot fill typed proxy config")
		return err
	}

	var existedProxyCfg *models.ProxyConfig
	existedProxyCfg, err = dao.NewQuery(c).GetProxyConfigByOriginClientIDAndName(userInfo, clientID, proxyCfg.Name)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		logger.Logger(c).WithError(err).Errorf("cannot get proxy config, id: [%s]", clientID)
		return err
	}

	if !overwrite && err == nil {
		logger.Logger(c).Errorf("proxy config already exists, client: [%s], server: [%s], proxy: [%s]", clientID, serverID, proxyCfg.Name)
		return fmt.Errorf("proxy config already exist")
	}

	reservationCreated, err := validateRemotePortAvailable(c, userInfo, serverID, clientID, clientEntity.OriginClientID, typedProxyCfg, proxyRecordID(existedProxyCfg))
	if err != nil {
		logger.Logger(c).WithError(err).Errorf("proxy remote port is unavailable, client: [%s], server: [%s], proxy: [%s]", clientID, serverID, proxyCfg.Name)
		return err
	}
	reservationProtocol, reservationPort, reservationHasPort := proxyRemotePortValue(typedProxyCfg)
	reservationCommitted := false
	if reservationCreated && reservationHasPort {
		resourceID := proxyReservationResourceID(serverID, clientID, clientEntity.OriginClientID, proxyCfg.Name)
		defer func() {
			if reservationCommitted {
				return
			}
			if releaseErr := dao.NewMutation(c).ReleasePort(userInfo, serverID, defs.CliTypeServer, reservationProtocol, reservationPort, models.PortReservationTypeProxy, resourceID); releaseErr != nil {
				logger.Logger(c).WithError(releaseErr).Warnf("cannot roll back proxy port reservation, client: [%s], proxy: [%s]", clientID, proxyCfg.Name)
			}
		}()
	}

	// update client config
	if oldCfg, err := clientEntity.GetConfigContent(); err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot get client config, id: [%s]", clientID)
		return err
	} else {
		oldCfg.Proxies = lo.Filter(oldCfg.Proxies, func(proxy v1.TypedProxyConfig, _ int) bool {
			return proxy.GetBaseConfig().Name != typedProxyCfg.GetBaseConfig().Name
		})
		oldCfg.Proxies = append(oldCfg.Proxies, typedProxyCfg)

		if err := clientEntity.SetConfigContent(*oldCfg); err != nil {
			logger.Logger(c).WithError(err).Errorf("cannot set client config, id: [%s]", clientID)
			return err
		}
	}

	rawCfg, err := clientEntity.MarshalJSONConfig()
	if err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot marshal client config, id: [%s]", clientID)
		return err
	}

	_, err = client.UpdateFrpcHander(c, &pb.UpdateFRPCRequest{
		ClientId: &clientEntity.ClientID,
		ServerId: &serverID,
		Config:   rawCfg,
		Comment:  &clientEntity.Comment,
		FrpsUrl:  &clientEntity.FrpsUrl,
	})
	if err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot update frpc failed, id: [%s]", clientID)
		return err
	}

	if existedProxyCfg != nil && existedProxyCfg.ServerID != serverID {
		logger.Logger(c).Warnf("client and server not match, delete old proxy, client: [%s], server: [%s], proxy: [%s]", clientID, serverID, proxyCfg.Name)
		if _, err := DeleteProxyConfig(c, &pb.DeleteProxyConfigRequest{
			ClientId: lo.ToPtr(existedProxyCfg.ClientID),
			ServerId: lo.ToPtr(existedProxyCfg.ServerID),
			Name:     &proxyCfg.Name,
		}); err != nil {
			logger.Logger(c).WithError(err).Errorf("cannot delete old proxy, client: [%s], server: [%s], proxy: [%s]", clientID, clientEntity.ServerID, proxyCfg.Name)
			return err
		}
	}
	reservationCommitted = true

	return nil
}

func proxyRecordID(proxyCfg *models.ProxyConfig) uint {
	if proxyCfg == nil || proxyCfg.Model == nil {
		return 0
	}
	return proxyCfg.ID
}
