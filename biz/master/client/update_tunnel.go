package client

import (
	"encoding/json"
	"fmt"
	"net/url"

	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/samber/lo"
	"github.com/spf13/cast"
	"github.com/yunj-yun/frp-web-panel/common"
	"github.com/yunj-yun/frp-web-panel/defs"
	"github.com/yunj-yun/frp-web-panel/models"
	"github.com/yunj-yun/frp-web-panel/pb"
	"github.com/yunj-yun/frp-web-panel/services/app"
	"github.com/yunj-yun/frp-web-panel/services/dao"
	"github.com/yunj-yun/frp-web-panel/services/rpc"
	"github.com/yunj-yun/frp-web-panel/utils"
	"github.com/yunj-yun/frp-web-panel/utils/logger"
)

func UpdateFrpcHander(c *app.Context, req *pb.UpdateFRPCRequest) (*pb.UpdateFRPCResponse, error) {
	logger.Logger(c).Infof("update frpc, clientID: [%s], serverID: [%s], configBytes: [%d]", req.GetClientId(), req.GetServerId(), len(req.GetConfig()))
	var (
		content     = req.GetConfig()
		serverID    = req.GetServerId()
		reqClientID = req.GetClientId() // may be shadow or child
		userInfo    = common.GetUserInfo(c)
	)
	q := dao.NewQuery(c)
	m := dao.NewMutation(c)

	cliCfg, err := utils.LoadClientConfigNormal(content, true)
	if err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot load config")
		return &pb.UpdateFRPCResponse{
			Status: &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: err.Error()},
		}, err
	}

	cliRecord, err := q.GetClientByClientID(userInfo, reqClientID)
	if err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot get client, id: [%s]", reqClientID)
		return &pb.UpdateFRPCResponse{
			Status: &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: "cannot get client"},
		}, fmt.Errorf("cannot get client")
	}

	cli := cliRecord.ClientEntity

	if cli.IsShadow {
		cli, _, err = ChildClientForServer(c, serverID, cli)
		if err != nil {
			logger.Logger(c).WithError(err).Errorf("cannot get child client, id: [%s]", reqClientID)
			return &pb.UpdateFRPCResponse{
				Status: &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: "cannot get child client"},
			}, fmt.Errorf("cannot get child client")
		}
	}

	if cli.IsShadow && len(cli.ConfigContent) == 0 {
		logger.Logger(c).Warnf("client is shadowed, cannot update, id: [%s]", reqClientID)
		return nil, fmt.Errorf("client is shadowed, cannot update")
	}

	if !cli.IsShadow && len(cli.OriginClientID) == 0 {
		logger.Logger(c).Warnf("client is not shadowed, make it shadow, id: [%s]", reqClientID)
		cli, err = MakeClientShadowed(c, serverID, cli)
		if err != nil {
			logger.Logger(c).WithError(err).Errorf("cannot make client shadowed, id: [%s]", reqClientID)
			return &pb.UpdateFRPCResponse{
				Status: &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: "cannot make client shadowed"},
			}, fmt.Errorf("cannot make client shadowed")
		}
	}

	srv, err := q.GetServerByServerID(userInfo, req.GetServerId())
	if err != nil || srv == nil {
		logger.Logger(c).WithError(err).Errorf("cannot get server, id: [%s]", req.GetServerId())
		return &pb.UpdateFRPCResponse{
			Status: &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: fmt.Sprintf("server %q was not found", req.GetServerId())},
		}, fmt.Errorf("cannot get server")
	}
	// The client config needs the frps address and bind port. Older Servers were
	// created with an empty config, so repair that here instead of failing: a
	// Server record is only useful once it carries usable frps defaults.
	if len(srv.ServerIP) == 0 {
		logger.Logger(c).Warnf("server has no address, id: [%s]", req.GetServerId())
		return &pb.UpdateFRPCResponse{
			Status: &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: fmt.Sprintf("server %q has no address yet; save the Server configuration first", req.GetServerId())},
		}, fmt.Errorf("server has no address")
	}
	if len(srv.ConfigContent) == 0 {
		logger.Logger(c).Warnf("server has no frps config, seeding defaults, id: [%s]", req.GetServerId())
		if err := srv.SetConfigContent(models.DefaultServerConfig()); err != nil {
			logger.Logger(c).WithError(err).Errorf("cannot seed server config, id: [%s]", req.GetServerId())
			return &pb.UpdateFRPCResponse{
				Status: &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: fmt.Sprintf("cannot initialize the frps configuration of server %q", req.GetServerId())},
			}, err
		}
		if err := m.UpdateServer(userInfo, srv); err != nil {
			logger.Logger(c).WithError(err).Errorf("cannot persist seeded server config, id: [%s]", req.GetServerId())
			return &pb.UpdateFRPCResponse{
				Status: &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: fmt.Sprintf("cannot initialize the frps configuration of server %q", req.GetServerId())},
			}, err
		}
	}

	srvConf, err := srv.GetConfigContent()
	if srvConf == nil || err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot read server config, id: [%s]", req.GetServerId())
		return &pb.UpdateFRPCResponse{
			Status: &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: fmt.Sprintf("cannot read the frps configuration of server %q", req.GetServerId())},
		}, err
	}

	cliCfg.ServerAddr = srv.ServerIP
	switch cliCfg.Transport.Protocol {
	case "kcp":
		cliCfg.ServerPort = srvConf.KCPBindPort
	case "quic":
		cliCfg.ServerPort = srvConf.QUICBindPort
	default:
		cliCfg.ServerPort = srvConf.BindPort
	}

	if len(req.GetFrpsUrl()) > 0 || len(cli.FrpsUrl) > 0 {
		// 有一个有就需要覆盖，优先请求的url
		var (
			parsedFrpsUrl *url.URL
			err           error
			urlToParse    string
		)
		if len(req.GetFrpsUrl()) > 0 {
			parsedFrpsUrl, err = ValidateFrpsUrl(req.GetFrpsUrl())
			if err != nil {
				logger.Logger(c).WithError(err).Errorf("invalid frps url, url: [%s]", req.GetFrpsUrl())
				return &pb.UpdateFRPCResponse{
					Status: &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: err.Error()},
				}, err
			}
			urlToParse = req.GetFrpsUrl()
		}

		if len(cli.FrpsUrl) > 0 && parsedFrpsUrl == nil {
			parsedFrpsUrl, err = ValidateFrpsUrl(cli.FrpsUrl)
			if err != nil {
				logger.Logger(c).WithError(err).Errorf("invalid old frps url, url: [%s]", cli.FrpsUrl)
				return &pb.UpdateFRPCResponse{
					Status: &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: err.Error()},
				}, err
			}
			urlToParse = cli.FrpsUrl
		}

		cliCfg.ServerAddr = parsedFrpsUrl.Hostname()
		cliCfg.ServerPort = cast.ToInt(parsedFrpsUrl.Port())
		cliCfg.Transport.Protocol = parsedFrpsUrl.Scheme
		cli.FrpsUrl = urlToParse
	}

	cliCfg.User = userInfo.GetUserName()

	if cliCfg.Metadatas == nil {
		cliCfg.Metadatas = make(map[string]string)
	}

	cliCfg.Metadatas[defs.FRPAuthTokenKey] = userInfo.GetToken()
	cliCfg.Metadatas[defs.FRPClientIDKey] = reqClientID

	newCfg := struct {
		v1.ClientCommonConfig
		Proxies  []v1.ProxyConfigurer   `json:"proxies,omitempty"`
		Visitors []v1.VisitorConfigurer `json:"visitors,omitempty"`
	}{
		ClientCommonConfig: cliCfg.ClientCommonConfig,
		Proxies:            lo.Map(cliCfg.Proxies, func(item v1.TypedProxyConfig, _ int) v1.ProxyConfigurer { return item.ProxyConfigurer }),
		Visitors:           lo.Map(cliCfg.Visitors, func(item v1.TypedVisitorConfig, _ int) v1.VisitorConfigurer { return item.VisitorConfigurer }),
	}

	rawCliConf, err := json.Marshal(newCfg)
	if err != nil {
		logger.Logger(c).WithError(err).Error("cannot marshal config")
		return nil, err
	}

	cli.ConfigContent = rawCliConf
	cli.ServerID = serverID
	if req.Comment != nil {
		cli.Comment = req.GetComment()
	}

	if err := m.UpdateClient(userInfo, cli); err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot update client, id: [%s]", cli.ClientID)
		return nil, err
	}

	if err := m.RebuildProxyConfigFromClient(userInfo, &models.Client{ClientEntity: cli}); err != nil {
		logger.Logger(c).WithError(err).Errorf("cannot rebuild proxy config from client, id: [%s]", cli.ClientID)
		return nil, err
	}

	cliReq := &pb.UpdateFRPCRequest{
		ClientId: lo.ToPtr(cli.ClientID),
		ServerId: lo.ToPtr(serverID),
		Config:   rawCliConf,
	}

	go func() {
		childCtx := c.Background()
		cliToUpdate, err := dao.NewQuery(childCtx).GetClientByFilter(userInfo, &models.ClientEntity{ClientID: cli.OriginClientID}, nil)
		if err != nil {
			logger.Logger(childCtx).WithError(err).Errorf("cannot get origin client, id: [%s]", cliToUpdate.OriginClientID)
			return
		}

		if cliToUpdate.Stopped {
			logger.Logger(childCtx).Infof("client [%s] is stopped, do not send update event", cliToUpdate.OriginClientID)
			return
		}

		resp, err := rpc.CallClient(childCtx, cliToUpdate.ClientID, pb.Event_EVENT_UPDATE_FRPC, cliReq)
		if err != nil {
			logger.Logger(childCtx).WithError(err).Errorf("update event send to client error, server: [%s], client: [%+v], updated client: [%+v]", serverID, cliToUpdate, cli)
		}

		if resp == nil {
			logger.Logger(childCtx).Errorf("cannot get response, server: [%s], client: [%s]", serverID, cliToUpdate.OriginClientID)
		}
	}()

	logger.Logger(c).Infof("update frpc success, client id: [%s]", reqClientID)
	return &pb.UpdateFRPCResponse{
		Status: &pb.Status{Code: pb.RespCode_RESP_CODE_SUCCESS, Message: "ok"},
	}, nil
}
