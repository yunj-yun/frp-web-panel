package certificate

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/yunj-yun/frp-web-panel/common"
	"github.com/yunj-yun/frp-web-panel/models"
	"github.com/yunj-yun/frp-web-panel/pb"
	"github.com/yunj-yun/frp-web-panel/services/app"
	"github.com/yunj-yun/frp-web-panel/services/dao"
	v1 "github.com/fatedier/frp/pkg/config/v1"
)

// gatewayDomainPattern intentionally accepts DNS names only.  Caddy can
// serve IP literals, but managed certificates cannot be issued for them and
// the gateway route is always backed by a managed certificate.
var gatewayDomainPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)

const defaultGatewayListenPort = uint32(443)

type gatewayRouteInput struct {
	domain        string
	listenPort    uint32
	serverID      string
	proxyID       uint
	certificateID uint
	upstreamPort  uint16
}

// ListGatewayRoutes returns routes scoped to the authenticated user's tenant.
func ListGatewayRoutes(ctx *app.Context, req *pb.ListGatewayRoutesRequest) (*pb.ListGatewayRoutesResponse, error) {
	userInfo := common.GetUserInfo(ctx)
	if userInfo == nil || !userInfo.Valid() {
		return gatewayRouteListError(pb.RespCode_RESP_CODE_UNAUTHORIZED, "user is not authorized"), nil
	}

	page, pageSize := certificatePage(req.GetPage(), req.GetPageSize())
	keyword := strings.TrimSpace(req.GetKeyword())
	serverID := strings.TrimSpace(req.GetServerId())
	query := dao.NewQuery(ctx)
	routes, err := query.ListGatewayRoutes(userInfo, page, pageSize, keyword, serverID)
	if err != nil {
		return nil, err
	}
	total, err := query.CountGatewayRoutes(userInfo, keyword, serverID)
	if err != nil {
		return nil, err
	}
	items := make([]*pb.GatewayRoute, 0, len(routes))
	for _, route := range routes {
		items = append(items, gatewayRouteToPB(route))
	}
	return &pb.ListGatewayRoutesResponse{
		Status: successStatus(),
		Routes: items,
		Total:  uint32(total),
	}, nil
}

// CreateGatewayRoute validates the server, certificate and HTTP FRP proxy
// before persisting a pending route.  Applying the route to Caddy is handled
// asynchronously by the gateway worker.
func CreateGatewayRoute(ctx *app.Context, req *pb.CreateGatewayRouteRequest) (*pb.CreateGatewayRouteResponse, error) {
	userInfo := common.GetUserInfo(ctx)
	if userInfo == nil || !userInfo.Valid() {
		return &pb.CreateGatewayRouteResponse{Status: status(pb.RespCode_RESP_CODE_UNAUTHORIZED, "user is not authorized")}, nil
	}
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	input, err := validateGatewayRouteReferences(ctx, userInfo, gatewayRouteInputFromCreate(req))
	if err != nil {
		return nil, err
	}
	if conflict, err := dao.NewQuery(ctx).FindGatewayRouteConflict(userInfo, input.domain, uint16(input.listenPort), 0); err != nil {
		return nil, err
	} else if conflict != nil {
		return nil, fmt.Errorf("gateway route for %s:%d already exists", input.domain, input.listenPort)
	}

	route := &models.GatewayRoute{
		Domain:        input.domain,
		ListenPort:    uint16(input.listenPort),
		ServerID:      input.serverID,
		ProxyID:       input.proxyID,
		CertificateID: input.certificateID,
		UpstreamPort:  input.upstreamPort,
		Status:        models.GatewayRouteStatusPending,
	}
	if err := dao.NewMutation(ctx).CreateGatewayRoute(userInfo, route); err != nil {
		return nil, err
	}
	triggerGatewaySync(ctx, userInfo, route.ServerID)
	return &pb.CreateGatewayRouteResponse{Status: successStatus(), Route: gatewayRouteToPB(route)}, nil
}

// UpdateGatewayRoute revalidates all references so a route cannot be moved
// across tenants, servers or certificates by changing only its ID.
func UpdateGatewayRoute(ctx *app.Context, req *pb.UpdateGatewayRouteRequest) (*pb.UpdateGatewayRouteResponse, error) {
	userInfo := common.GetUserInfo(ctx)
	if userInfo == nil || !userInfo.Valid() {
		return &pb.UpdateGatewayRouteResponse{Status: status(pb.RespCode_RESP_CODE_UNAUTHORIZED, "user is not authorized")}, nil
	}
	if req == nil || req.GetId() == 0 {
		return nil, fmt.Errorf("gateway route id is required")
	}

	query := dao.NewQuery(ctx)
	existing, err := query.GetGatewayRoute(userInfo, uint(req.GetId()))
	if err != nil {
		return nil, err
	}
	input, err := validateGatewayRouteReferences(ctx, userInfo, gatewayRouteInputFromUpdate(req))
	if err != nil {
		return nil, err
	}
	if conflict, err := query.FindGatewayRouteConflict(userInfo, input.domain, uint16(input.listenPort), existing.ID); err != nil {
		return nil, err
	} else if conflict != nil {
		return nil, fmt.Errorf("gateway route for %s:%d already exists", input.domain, input.listenPort)
	}

	oldServerID := existing.ServerID
	existing.Domain = input.domain
	existing.ListenPort = uint16(input.listenPort)
	existing.ServerID = input.serverID
	existing.ProxyID = input.proxyID
	existing.CertificateID = input.certificateID
	existing.UpstreamPort = input.upstreamPort
	existing.Status = models.GatewayRouteStatusPending
	existing.ErrorMessage = ""
	existing.LastAppliedAt = nil
	if err := dao.NewMutation(ctx).UpdateGatewayRoute(userInfo, existing); err != nil {
		return nil, err
	}
	triggerGatewaySync(ctx, userInfo, existing.ServerID)
	if oldServerID != existing.ServerID {
		triggerGatewaySync(ctx, userInfo, oldServerID)
	}
	return &pb.UpdateGatewayRouteResponse{Status: successStatus(), Route: gatewayRouteToPB(existing)}, nil
}

func DeleteGatewayRoute(ctx *app.Context, req *pb.DeleteGatewayRouteRequest) (*pb.DeleteGatewayRouteResponse, error) {
	userInfo := common.GetUserInfo(ctx)
	if userInfo == nil || !userInfo.Valid() {
		return &pb.DeleteGatewayRouteResponse{Status: status(pb.RespCode_RESP_CODE_UNAUTHORIZED, "user is not authorized")}, nil
	}
	if req == nil || req.GetId() == 0 {
		return nil, fmt.Errorf("gateway route id is required")
	}
	route, err := dao.NewQuery(ctx).GetGatewayRoute(userInfo, uint(req.GetId()))
	if err != nil {
		return nil, err
	}
	if err := dao.NewMutation(ctx).DeleteGatewayRoute(userInfo, uint(req.GetId())); err != nil {
		return nil, err
	}
	triggerGatewaySync(ctx, userInfo, route.ServerID)
	return &pb.DeleteGatewayRouteResponse{Status: successStatus()}, nil
}

// SyncGatewayRoute revalidates a route and puts it back into the pending
// state.  The worker that applies Caddy configuration observes this state and
// reports active/error after the remote operation completes.
func SyncGatewayRoute(ctx *app.Context, req *pb.SyncGatewayRouteRequest) (*pb.SyncGatewayRouteResponse, error) {
	userInfo := common.GetUserInfo(ctx)
	if userInfo == nil || !userInfo.Valid() {
		return &pb.SyncGatewayRouteResponse{Status: status(pb.RespCode_RESP_CODE_UNAUTHORIZED, "user is not authorized")}, nil
	}
	if req == nil || req.GetId() == 0 {
		return nil, fmt.Errorf("gateway route id is required")
	}

	query := dao.NewQuery(ctx)
	route, err := query.GetGatewayRoute(userInfo, uint(req.GetId()))
	if err != nil {
		return nil, err
	}
	input, err := validateGatewayRouteReferences(ctx, userInfo, gatewayRouteInput{
		domain:        route.Domain,
		listenPort:    uint32(route.ListenPort),
		serverID:      route.ServerID,
		proxyID:       route.ProxyID,
		certificateID: route.CertificateID,
	})
	if err != nil {
		return nil, err
	}
	route.Domain = input.domain
	route.ListenPort = uint16(input.listenPort)
	route.UpstreamPort = input.upstreamPort
	route.Status = models.GatewayRouteStatusPending
	route.ErrorMessage = ""
	route.LastAppliedAt = nil
	if err := dao.NewMutation(ctx).UpdateGatewayRoute(userInfo, route); err != nil {
		return nil, err
	}
	triggerGatewaySync(ctx, userInfo, route.ServerID)
	return &pb.SyncGatewayRouteResponse{Status: successStatus(), Route: gatewayRouteToPB(route)}, nil
}

func gatewayRouteInputFromCreate(req *pb.CreateGatewayRouteRequest) gatewayRouteInput {
	return gatewayRouteInput{
		domain:        req.GetDomain(),
		listenPort:    req.GetListenPort(),
		serverID:      req.GetServerId(),
		proxyID:       uint(req.GetProxyId()),
		certificateID: uint(req.GetCertificateId()),
	}
}

func gatewayRouteInputFromUpdate(req *pb.UpdateGatewayRouteRequest) gatewayRouteInput {
	return gatewayRouteInput{
		domain:        req.GetDomain(),
		listenPort:    req.GetListenPort(),
		serverID:      req.GetServerId(),
		proxyID:       uint(req.GetProxyId()),
		certificateID: uint(req.GetCertificateId()),
	}
}

func validateGatewayRouteReferences(ctx *app.Context, userInfo models.UserInfo, input gatewayRouteInput) (gatewayRouteInput, error) {
	domain, err := normalizeGatewayDomain(input.domain)
	if err != nil {
		return gatewayRouteInput{}, err
	}
	listenPort, err := normalizeGatewayPort(uint32(input.listenPort))
	if err != nil {
		return gatewayRouteInput{}, err
	}
	input.domain = domain
	input.listenPort = uint32(listenPort)
	input.serverID = strings.TrimSpace(input.serverID)
	if input.serverID == "" {
		return gatewayRouteInput{}, fmt.Errorf("server id is required")
	}
	if input.proxyID == 0 {
		return gatewayRouteInput{}, fmt.Errorf("proxy id is required")
	}
	if input.certificateID == 0 {
		return gatewayRouteInput{}, fmt.Errorf("certificate id is required")
	}

	query := dao.NewQuery(ctx)
	server, err := query.GetServerByServerID(userInfo, input.serverID)
	if err != nil {
		return gatewayRouteInput{}, fmt.Errorf("server is not available: %w", err)
	}
	serverConfig, err := server.GetConfigContent()
	if err != nil {
		return gatewayRouteInput{}, fmt.Errorf("server configuration is unavailable: %w", err)
	}
	if serverConfig.VhostHTTPPort <= 0 || serverConfig.VhostHTTPPort > 65535 {
		return gatewayRouteInput{}, fmt.Errorf("server vhostHTTPPort must be configured before creating a gateway route")
	}
	input.upstreamPort = uint16(serverConfig.VhostHTTPPort)
	if input.listenPort == uint32(input.upstreamPort) {
		return gatewayRouteInput{}, fmt.Errorf("listen port conflicts with server vhostHTTPPort")
	}
	if reserved := serverReservedPort(input.listenPort, serverConfig); reserved != "" {
		return gatewayRouteInput{}, fmt.Errorf("listen port %d conflicts with server %s", input.listenPort, reserved)
	}
	if conflict, err := findObservedPortConflict(query, userInfo, input.serverID, uint16(input.listenPort)); err != nil {
		return gatewayRouteInput{}, err
	} else if conflict != "" {
		return gatewayRouteInput{}, fmt.Errorf("listen port %d is currently used by %s", input.listenPort, conflict)
	}

	certificate, err := query.GetManagedCertificate(userInfo, input.certificateID)
	if err != nil {
		return gatewayRouteInput{}, fmt.Errorf("certificate is not available: %w", err)
	}
	if certificate.ServerID != input.serverID {
		return gatewayRouteInput{}, fmt.Errorf("certificate belongs to a different server")
	}
	switch certificate.Status {
	case models.ManagedCertificateStatusPending, models.ManagedCertificateStatusRenewing, models.ManagedCertificateStatusReady:
		// Caddy obtains a pending certificate as part of the first desired
		// configuration. The route can therefore be created before ACME has
		// completed; its status remains pending until the Agent applies it.
	case models.ManagedCertificateStatusExpired, models.ManagedCertificateStatusError, models.ManagedCertificateStatusRevoked:
		return gatewayRouteInput{}, fmt.Errorf("certificate is not usable (%s)", certificate.Status)
	default:
		return gatewayRouteInput{}, fmt.Errorf("certificate is not usable")
	}
	if !certificateCoversDomain(certificate, domain) {
		return gatewayRouteInput{}, fmt.Errorf("certificate does not cover domain %q", domain)
	}

	proxyConfig, err := query.GetProxyConfigByID(userInfo, input.proxyID)
	if err != nil {
		return gatewayRouteInput{}, fmt.Errorf("proxy is not available: %w", err)
	}
	if proxyConfig.ServerID != input.serverID {
		return gatewayRouteInput{}, fmt.Errorf("proxy belongs to a different server")
	}
	if proxyConfig.Stopped {
		return gatewayRouteInput{}, fmt.Errorf("proxy is stopped")
	}
	typedProxy, err := proxyConfig.GetTypedProxyConfig()
	if err != nil {
		return gatewayRouteInput{}, fmt.Errorf("proxy configuration is invalid: %w", err)
	}
	if typedProxy.Type != string(v1.ProxyTypeHTTP) {
		return gatewayRouteInput{}, fmt.Errorf("gateway routes require an HTTP proxy")
	}
	httpProxy, ok := typedProxy.ProxyConfigurer.(*v1.HTTPProxyConfig)
	if !ok || httpProxy == nil {
		return gatewayRouteInput{}, fmt.Errorf("proxy configuration is not an HTTP proxy")
	}
	if !containsGatewayDomain(httpProxy.CustomDomains, domain) && !containsGatewaySubdomain(httpProxy.SubDomain, serverConfig.SubDomainHost, domain) {
		return gatewayRouteInput{}, fmt.Errorf("HTTP proxy does not expose domain %q", domain)
	}
	return input, nil
}

func serverReservedPort(port uint32, config *v1.ServerConfig) string {
	return models.FRPServerPortReservationConflict(config, "tcp", uint16(port))
}

func findObservedPortConflict(query dao.PortSnapshotQuery, userInfo models.UserInfo, serverID string, port uint16) (string, error) {
	if query == nil || port == 0 {
		return "", nil
	}
	scans, err := query.GetNodePortScans(userInfo, nil, []string{serverID})
	if err != nil {
		return "", fmt.Errorf("check observed server ports: %w", err)
	}
	if len(scans) == 0 {
		return "", nil
	}
	scanIDs := make([]uint, 0, len(scans))
	for _, scan := range scans {
		if scan != nil {
			scanIDs = append(scanIDs, scan.ID)
		}
	}
	sockets, err := query.GetNodePortSockets(scanIDs)
	if err != nil {
		return "", fmt.Errorf("check observed server ports: %w", err)
	}
	var observed []string
	allCaddy := true
	for _, socket := range sockets {
		if socket == nil || socket.Port != port || socket.Protocol != "tcp" {
			continue
		}
		process := strings.TrimSpace(socket.Process)
		if process == "" || !strings.Contains(strings.ToLower(process), "caddy") {
			allCaddy = false
		}
		observed = append(observed, fmt.Sprintf("%s %s:%d", socket.Protocol, socket.Address, socket.Port))
	}
	if len(observed) == 0 || ((port == models.CaddyHTTPPort || port == models.CaddyHTTPSPort) && allCaddy) {
		return "", nil
	}
	return strings.Join(observed, ", "), nil
}

func triggerGatewaySync(ctx *app.Context, userInfo models.UserInfo, serverID string) {
	if ctx == nil || ctx.GetApp() == nil || userInfo == nil || strings.TrimSpace(serverID) == "" {
		return
	}
	// The HTTP API should acknowledge persistence immediately. The background
	// sync uses a fresh context because the request-scoped Gin context may be
	// cancelled as soon as the response is written.
	appInstance := ctx.GetApp()
	scope := userInfo.GetSafeUserInfo()
	go func() {
		if err := SyncServerCaddy(appInstance, &scope, serverID); err != nil {
			appInstance.Logger(context.Background()).WithError(err).Warnf("sync Caddy gateway routes for server %s", serverID)
		}
	}()
}

func normalizeGatewayDomain(raw string) (string, error) {
	domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
	if domain == "" {
		return "", fmt.Errorf("domain is required")
	}
	if len(domain) > 253 || net.ParseIP(domain) != nil || !gatewayDomainPattern.MatchString(domain) {
		return "", fmt.Errorf("invalid domain %q", raw)
	}
	return domain, nil
}

func normalizeGatewayPort(port uint32) (uint16, error) {
	if port == 0 {
		port = defaultGatewayListenPort
	}
	if port > 65535 {
		return 0, fmt.Errorf("listen port must be between 1 and 65535")
	}
	return uint16(port), nil
}

func certificateCoversDomain(certificate *models.ManagedCertificate, domain string) bool {
	if certificate == nil {
		return false
	}
	domains := append([]string(nil), certificate.Domains...)
	if certificate.PrimaryDomain != "" {
		domains = append(domains, certificate.PrimaryDomain)
	}
	for _, raw := range domains {
		candidate := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
		if candidate == domain {
			return true
		}
		// Existing records may predate wildcard validation.  A wildcard covers
		// exactly one label, matching ACME certificate semantics.
		if strings.HasPrefix(candidate, "*.") && strings.Count(domain, ".") == strings.Count(candidate, ".") && strings.HasSuffix(domain, candidate[1:]) {
			return true
		}
	}
	return false
}

func containsGatewayDomain(domains []string, domain string) bool {
	for _, raw := range domains {
		candidate := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
		if candidate == domain {
			return true
		}
	}
	return false
}

func containsGatewaySubdomain(subdomain, subdomainHost, domain string) bool {
	subdomain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(subdomain), "."))
	subdomainHost = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(subdomainHost), "."))
	if subdomain == "" || subdomainHost == "" || strings.Contains(subdomain, ".") {
		return false
	}
	return subdomain+"."+subdomainHost == domain
}

func gatewayRouteListError(code pb.RespCode, message string) *pb.ListGatewayRoutesResponse {
	return &pb.ListGatewayRoutesResponse{Status: status(code, message), Routes: []*pb.GatewayRoute{}}
}

func gatewayRouteToPB(route *models.GatewayRoute) *pb.GatewayRoute {
	if route == nil {
		return nil
	}
	item := &pb.GatewayRoute{
		Id:            uint32(route.ID),
		Domain:        route.Domain,
		ListenPort:    uint32(route.ListenPort),
		ServerId:      route.ServerID,
		ProxyId:       uint32(route.ProxyID),
		CertificateId: uint32(route.CertificateID),
		UpstreamPort:  uint32(route.UpstreamPort),
		Status:        route.Status,
		ErrorMessage:  route.ErrorMessage,
		CreatedAt:     route.CreatedAt.UnixMilli(),
		UpdatedAt:     route.UpdatedAt.UnixMilli(),
	}
	if route.LastAppliedAt != nil {
		item.LastAppliedAt = route.LastAppliedAt.UnixMilli()
	}
	return item
}
