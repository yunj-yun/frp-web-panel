package shared

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sourcegraph/conc"
	"github.com/yunj-yun/frp-web-panel/biz/master/auth"
	"github.com/yunj-yun/frp-web-panel/biz/master/certificate"
	"github.com/yunj-yun/frp-web-panel/biz/master/proxy"
	"github.com/yunj-yun/frp-web-panel/conf"
	"github.com/yunj-yun/frp-web-panel/services/app"
	"github.com/yunj-yun/frp-web-panel/services/cache"
	"github.com/yunj-yun/frp-web-panel/services/master"
	"github.com/yunj-yun/frp-web-panel/services/mux"
	"github.com/yunj-yun/frp-web-panel/services/watcher"
	"github.com/yunj-yun/frp-web-panel/utils/logger"
	"github.com/yunj-yun/frp-web-panel/utils/wsgrpc"
	"go.uber.org/fx"
)

type runMasterParam struct {
	fx.In

	Lc fx.Lifecycle

	Ctx                 *app.Context
	AppInstance         app.Application
	DBManagerMgr        app.DBManager
	HTTPMuxServer       mux.MuxServer `name:"httpMux"`
	TLSMuxServer        mux.MuxServer `name:"tlsMux"`
	MasterRouter        *gin.Engine   `name:"masterRouter"`
	ClientLogManager    app.ClientLogManager
	WsGrpcHandler       gin.HandlerFunc      `name:"wsGrpcHandler"`
	MasterService       master.MasterService `name:"wsMasterService"`
	TaskManager         watcher.Client       `name:"masterTaskManager"`
	WsListener          *wsgrpc.WSListener
	DefaultServerConfig conf.Config `name:"defaultServerConfig"`
	PermManager         app.PermissionManager
}

func runMaster(param runMasterParam) {

	param.AppInstance.SetClientLogManager(param.ClientLogManager)
	param.MasterRouter.GET("/wsgrpc", param.WsGrpcHandler)

	cache.InitCache(param.AppInstance.GetConfig())
	auth.InitAuth(param.AppInstance)

	param.TaskManager.AddCronTask("0 0 3 * * *", proxy.CollectDailyStats, param.AppInstance)
	// Caddy owns the actual ACME exchange on each Server. The Master scheduler
	// only claims queued jobs and sends an idempotent desired Caddyfile through
	// the authenticated control stream.
	param.TaskManager.AddDurationTask(30*time.Second, certificate.ProcessQueuedJobs, param.AppInstance)
	param.TaskManager.AddDurationTask(60*time.Second, certificate.RefreshCertificateStatuses, param.AppInstance)

	logger.Logger(param.Ctx).Infof("start to run master")
	var wg conc.WaitGroup

	param.Lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			wg.Go(func() {
				if err := param.MasterService.GetServer().Serve(param.WsListener); err != nil {
					logger.Logger(param.Ctx).Fatalf("gRPC server error: %v", err)
				}
			})
			wg.Go(param.TLSMuxServer.Run)
			wg.Go(param.HTTPMuxServer.Run)
			wg.Go(param.TaskManager.Run)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			param.MasterService.GetServer().Stop()
			param.TLSMuxServer.Stop()
			param.HTTPMuxServer.Stop()
			param.TaskManager.Stop()
			wg.Wait()
			return nil
		},
	})
}
