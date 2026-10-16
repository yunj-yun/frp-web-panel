package certificate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/VaalaCat/frp-panel/defs"
	"github.com/VaalaCat/frp-panel/models"
	"github.com/VaalaCat/frp-panel/pb"
	"github.com/VaalaCat/frp-panel/services/app"
	"github.com/VaalaCat/frp-panel/services/caddy"
	"github.com/VaalaCat/frp-panel/services/dao"
	"github.com/VaalaCat/frp-panel/services/rpc"
	"github.com/VaalaCat/frp-panel/utils/logger"
	"gorm.io/gorm"
)

const certificateJobBatchSize = 16

type certificateServerScope struct {
	userID   int
	tenantID int
	serverID string
}

// ProcessQueuedJobs applies a bounded batch of certificate requests. It is
// called by the Master scheduler and is deliberately idempotent: claiming a
// job uses a conditional database update, while the Agent compares the
// configuration hash before touching Caddy.
func ProcessQueuedJobs(appInstance app.Application) {
	if appInstance == nil || appInstance.GetDBManager() == nil {
		return
	}
	ctx := app.NewContext(context.Background(), appInstance)
	jobs, err := dao.NewQuery(ctx).AdminListRunnableCertificateJobs(certificateJobBatchSize)
	if err != nil {
		logger.Logger(ctx).WithError(err).Error("list queued certificate jobs")
		return
	}
	for _, job := range jobs {
		if job == nil {
			continue
		}
		if err := processJob(ctx, job.ID); err != nil {
			logger.Logger(ctx).WithError(err).Errorf("process certificate job %d", job.ID)
		}
	}
}

// RefreshCertificateStatuses asks every online Server Agent for the public
// certificate metadata Caddy currently has on disk. This runs independently
// of issuance jobs because an ACME challenge can complete well after the
// desired Caddyfile was first loaded.
func RefreshCertificateStatuses(appInstance app.Application) {
	if appInstance == nil || appInstance.GetDBManager() == nil {
		return
	}
	ctx := app.NewContext(context.Background(), appInstance)
	certificates, err := dao.NewQuery(ctx).AdminListManagedCertificates()
	if err != nil {
		logger.Logger(ctx).WithError(err).Error("list managed certificates for status refresh")
		return
	}
	grouped := make(map[certificateServerScope][]*models.ManagedCertificate)
	for _, certificate := range certificates {
		if certificate == nil || strings.TrimSpace(certificate.ServerID) == "" {
			continue
		}
		scope := certificateServerScope{userID: certificate.UserID, tenantID: certificate.TenantID, serverID: certificate.ServerID}
		grouped[scope] = append(grouped[scope], certificate)
	}
	for scope, items := range grouped {
		server, err := dao.NewQuery(ctx).AdminGetServerByServerID(scope.serverID)
		if err != nil || server.UserID != scope.userID || server.TenantID != scope.tenantID || appInstance.GetClientsManager().Get(scope.serverID) == nil {
			continue
		}
		response := &pb.GetCaddyStatusResponse{}
		if err := rpc.CallClientWrapper(ctx, scope.serverID, pb.Event_EVENT_GET_CADDY_STATUS, &pb.GetCaddyStatusRequest{
			Base: &pb.ServerBase{ServerId: scope.serverID, ServerSecret: server.ConnectSecret},
		}, response); err != nil {
			logger.Logger(ctx).WithError(err).Debugf("refresh Caddy certificate status on server %s", scope.serverID)
			continue
		}
		if response.GetStatus().GetCode() != pb.RespCode_RESP_CODE_SUCCESS {
			continue
		}
		userInfo := &models.UserEntity{UserID: scope.userID, TenantID: scope.tenantID, Status: models.STATUS_NORMAL}
		applyCertificateMetadata(ctx, userInfo, items, response.GetCertificates())
	}
}

func processJob(ctx *app.Context, jobID uint) error {
	mutation := dao.NewMutation(ctx)
	job, err := mutation.AdminClaimCertificateJob(jobID, time.Now())
	if err != nil {
		// A second scheduler tick may have claimed the job already. Treat that
		// as a normal race rather than changing its state.
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	query := dao.NewQuery(ctx)
	certificate, err := query.AdminGetManagedCertificate(job.CertificateID)
	if err != nil {
		return completeJob(ctx, job, fmt.Errorf("load certificate: %w", err))
	}
	if certificate.UserID != job.UserID || certificate.TenantID != job.TenantID {
		return completeJob(ctx, job, fmt.Errorf("certificate ownership changed"))
	}
	userInfo := &models.UserEntity{UserID: certificate.UserID, TenantID: certificate.TenantID, Status: models.STATUS_NORMAL}
	if err := SyncServerCaddy(ctx.GetApp(), userInfo, certificate.ServerID); err != nil {
		return completeJob(ctx, job, err)
	}
	return completeJob(ctx, job, nil)
}

// SyncServerCaddy renders and applies the complete desired configuration for
// one Server. It is shared by certificate jobs and gateway route mutations so
// a newly created route is applied even when no certificate job is pending.
func SyncServerCaddy(appInstance app.Application, userInfo models.UserInfo, serverID string) error {
	if appInstance == nil || userInfo == nil || !userInfo.Valid() {
		return fmt.Errorf("invalid sync context")
	}
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		return fmt.Errorf("server id is required")
	}
	ctx := app.NewContext(context.Background(), appInstance)
	query := dao.NewQuery(ctx)
	server, err := query.GetServerByServerID(userInfo, serverID)
	if err != nil {
		return fmt.Errorf("load target server: %w", err)
	}
	// The built-in default server predates tenant fields, so it is available
	// to the default administrator through the normal API exception.
	if serverID != defs.DefaultServerID && (server.UserID != userInfo.GetUserID() || server.TenantID != userInfo.GetTenantID()) {
		return fmt.Errorf("target server is outside certificate tenant")
	}
	routes, err := query.ListGatewayRoutesByServer(userInfo, serverID)
	if err != nil {
		return fmt.Errorf("list gateway routes: %w", err)
	}
	active := activeRoutes(routes)
	certificates, err := query.AdminListManagedCertificatesByServer(userInfo.GetUserID(), userInfo.GetTenantID(), serverID)
	if err != nil {
		return fmt.Errorf("list managed certificates: %w", err)
	}
	config, err := caddy.RenderGatewayCaddyfileWithCertificates(active, certificates)
	if err != nil {
		markGatewayRoutes(ctx, userInfo, routes, models.GatewayRouteStatusError, err.Error(), nil)
		return fmt.Errorf("render Caddyfile: %w", err)
	}
	if appInstance.GetClientsManager().Get(serverID) == nil {
		err := fmt.Errorf("target server is offline")
		markGatewayRoutes(ctx, userInfo, routes, models.GatewayRouteStatusError, err.Error(), nil)
		return err
	}
	response := &pb.UpdateCaddyResponse{}
	if err := rpc.CallClientWrapper(ctx, serverID, pb.Event_EVENT_UPDATE_CADDY, &pb.UpdateCaddyRequest{
		Caddyfile:  config,
		ConfigHash: caddy.ConfigHash(config),
		Base:       &pb.ServerBase{ServerId: serverID, ServerSecret: server.ConnectSecret},
	}, response); err != nil {
		markGatewayRoutes(ctx, userInfo, routes, models.GatewayRouteStatusError, err.Error(), nil)
		return fmt.Errorf("apply Caddy configuration: %w", err)
	}
	if response.GetStatus().GetCode() != pb.RespCode_RESP_CODE_SUCCESS {
		message := strings.TrimSpace(response.GetStatus().GetMessage())
		if message == "" {
			message = "Caddy rejected configuration"
		}
		markGatewayRoutes(ctx, userInfo, routes, models.GatewayRouteStatusError, message, nil)
		return fmt.Errorf("%s", message)
	}
	applyCertificateMetadata(ctx, userInfo, certificates, response.GetCertificates())
	now := time.Now()
	markGatewayRoutes(ctx, userInfo, active, models.GatewayRouteStatusActive, "", &now)
	return nil
}

func applyCertificateMetadata(ctx *app.Context, userInfo models.UserInfo, certificates []*models.ManagedCertificate, reported []*pb.CaddyCertificateStatus) {
	if len(certificates) == 0 || len(reported) == 0 {
		return
	}
	mutation := dao.NewMutation(ctx)
	for _, certificate := range certificates {
		if certificate == nil {
			continue
		}
		item := findCertificateStatus(certificate, reported)
		if item == nil {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(item.GetStatus()))
		switch status {
		case models.ManagedCertificateStatusReady, models.ManagedCertificateStatusExpired, models.ManagedCertificateStatusError:
		default:
			continue
		}
		var issuedAt, expiresAt *time.Time
		if item.GetIssuedAt() > 0 {
			value := time.UnixMilli(item.GetIssuedAt())
			issuedAt = &value
		}
		if item.GetExpiresAt() > 0 {
			value := time.UnixMilli(item.GetExpiresAt())
			expiresAt = &value
		}
		if err := mutation.AdminUpdateManagedCertificateStatus(certificate.ID, certificate.UserID, certificate.TenantID, status, item.GetErrorMessage(), item.GetIssuer(), item.GetFingerprint(), item.GetStorageId(), issuedAt, expiresAt); err != nil {
			logger.Logger(ctx).WithError(err).Warnf("update certificate %d metadata", certificate.ID)
		}
	}
}

func findCertificateStatus(certificate *models.ManagedCertificate, reported []*pb.CaddyCertificateStatus) *pb.CaddyCertificateStatus {
	if certificate == nil {
		return nil
	}
	domains := append([]string{certificate.PrimaryDomain}, certificate.Domains...)
	for _, candidate := range domains {
		candidate = normalizeCertificateDomain(candidate)
		if candidate == "" {
			continue
		}
		for _, item := range reported {
			if item == nil {
				continue
			}
			reportedDomain := normalizeCertificateDomain(item.GetDomain())
			if reportedDomain == candidate || (strings.HasPrefix(reportedDomain, "*.") && strings.HasSuffix(candidate, reportedDomain[1:])) {
				return item
			}
		}
	}
	return nil
}

func normalizeCertificateDomain(domain string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
}

func markGatewayRoutes(ctx *app.Context, userInfo models.UserInfo, routes []*models.GatewayRoute, status, message string, appliedAt *time.Time) {
	mutation := dao.NewMutation(ctx)
	for _, route := range routes {
		if route == nil || route.Status == models.GatewayRouteStatusDisabled {
			continue
		}
		route.Status = status
		route.ErrorMessage = strings.TrimSpace(message)
		route.LastAppliedAt = appliedAt
		if err := mutation.UpdateGatewayRoute(userInfo, route); err != nil {
			logger.Logger(ctx).WithError(err).Warnf("update gateway route %d sync status", route.ID)
		}
	}
}

func activeRoutes(routes []*models.GatewayRoute) []*models.GatewayRoute {
	active := make([]*models.GatewayRoute, 0, len(routes))
	for _, route := range routes {
		// Error and pending routes must remain eligible for a later retry. Only
		// explicitly disabled routes are omitted from the desired Caddyfile.
		if route == nil || route.Status == models.GatewayRouteStatusDisabled {
			continue
		}
		active = append(active, route)
	}
	return active
}

func completeJob(ctx *app.Context, job *models.CertificateJob, cause error) error {
	if job == nil {
		return cause
	}
	status := models.CertificateJobStatusSuccess
	message := ""
	if cause != nil {
		status = models.CertificateJobStatusFailed
		message = cause.Error()
	}
	// Do not manufacture an issuance timestamp merely because Caddy accepted a
	// configuration. The Server Agent reports the actual certificate's NotBefore
	// and NotAfter values separately after scanning Caddy storage.
	if err := dao.NewMutation(ctx).AdminCompleteCertificateJob(job.ID, job.CertificateID, job.UserID, job.TenantID, status, message, nil); err != nil {
		if cause != nil {
			return fmt.Errorf("%w; persist failure: %v", cause, err)
		}
		return fmt.Errorf("persist certificate result: %w", err)
	}
	return cause
}
