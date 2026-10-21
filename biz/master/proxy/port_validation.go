package proxy

import (
	"fmt"
	"strings"

	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/yunj-yun/frp-web-panel/defs"
	"github.com/yunj-yun/frp-web-panel/models"
	"github.com/yunj-yun/frp-web-panel/services/app"
	"github.com/yunj-yun/frp-web-panel/services/dao"
	"github.com/yunj-yun/frp-web-panel/utils/logger"
)

// validateRemotePortAvailable checks the port that a TCP or UDP proxy asks
// frps to bind.  The check deliberately combines three sources: frps' own
// configured listeners, the latest host socket snapshot, and the platform's
// persisted proxy records.  Socket snapshots are observations and can race
// with a process starting, so the final bind result from frps remains the
// authoritative check at apply time.
func validateRemotePortAvailable(
	c *app.Context,
	userInfo models.UserInfo,
	serverID, clientID, originClientID string,
	candidate v1.TypedProxyConfig,
	excludedProxyID uint,
) (bool, error) {
	protocol, port, err := proxyRemotePort(candidate)
	if err != nil {
		return false, err
	}
	if protocol == "" || port == 0 {
		return false, nil
	}
	if c == nil || userInfo == nil || !userInfo.Valid() {
		return false, fmt.Errorf("user info is required to validate proxy port")
	}

	query := dao.NewQuery(c)
	server, err := query.GetServerByServerID(userInfo, serverID)
	if err != nil {
		return false, fmt.Errorf("get server for port validation: %w", err)
	}
	// A Server created but not yet configured has no frps config stored, so its
	// reserved listeners (bindPort, vhostHTTPPort, ...) are unknown. Refusing to
	// create the tunnel here would block the normal setup order where users add
	// a Server and a tunnel before saving the frps config. Skip only the
	// configured-listener comparison; proxy records and port reservations are
	// still enforced below, and frps rejects a conflicting bind at apply time.
	serverConfig, err := server.GetConfigContent()
	if err != nil || serverConfig == nil {
		logger.Logger(c).WithError(err).Warnf("skip configured listener check, server configuration is unavailable, server: [%s]", serverID)
		serverConfig = nil
	}

	if reserved := proxyReservedServerPort(protocol, port, serverConfig); reserved != "" {
		return false, fmt.Errorf("%s proxy remote port %d conflicts with %s", protocol, port, reserved)
	}

	resourceID := proxyReservationResourceID(serverID, clientID, originClientID, candidate.GetBaseConfig().Name)
	reservationOwnsPort := false
	var reservationConflict *models.PortReservation
	reservations, err := query.ListActivePortReservations(userInfo, serverID, defs.CliTypeServer)
	if err != nil {
		return false, fmt.Errorf("check reserved server ports: %w", err)
	}
	for _, reservation := range reservations {
		if reservation == nil || reservation.Protocol != protocol || reservation.Port != port {
			continue
		}
		if reservation.ResourceType == models.PortReservationTypeProxy && reservation.ResourceID == resourceID {
			reservationOwnsPort = true
			continue
		}
		reservationConflict = reservation
	}

	proxies, err := query.ListProxyConfigsByServerID(userInfo, serverID)
	if err != nil {
		return false, fmt.Errorf("check configured proxy ports: %w", err)
	}
	configuredProxyOwnsPort := false
	reservationAliases := make([]string, 0, 1)
	for _, existing := range proxies {
		if existing == nil || existing.Stopped || existing.ID == excludedProxyID {
			if existing != nil && existing.ID == excludedProxyID {
				if existingCfg, cfgErr := existing.GetTypedProxyConfig(); cfgErr == nil {
					existingProtocol, existingPort, ok := proxyRemotePortValue(existingCfg)
					ownsPort := ok && existingProtocol == protocol && existingPort == port
					configuredProxyOwnsPort = configuredProxyOwnsPort || ownsPort
					if ownsPort {
						reservationAliases = append(reservationAliases, proxyReservationResourceID(existing.ServerID, existing.ClientID, existing.OriginClientID, existing.Name))
					}
				}
			}
			continue
		}
		// A shadow/client rebuild can leave the same logical proxy represented
		// by a child record.  Treat it as the record being replaced when its
		// identity and name match the candidate.
		if existing.Name == candidate.GetBaseConfig().Name && sameProxyClient(existing, clientID, originClientID) {
			existingCfg, cfgErr := existing.GetTypedProxyConfig()
			if cfgErr == nil {
				existingProtocol, existingPort, ok := proxyRemotePortValue(existingCfg)
				ownsPort := ok && existingProtocol == protocol && existingPort == port
				configuredProxyOwnsPort = configuredProxyOwnsPort || ownsPort
				if ownsPort {
					reservationAliases = append(reservationAliases, proxyReservationResourceID(existing.ServerID, existing.ClientID, existing.OriginClientID, existing.Name))
				}
			}
			continue
		}
		existingCfg, err := existing.GetTypedProxyConfig()
		if err != nil {
			return false, fmt.Errorf("read existing proxy %q for port validation: %w", existing.Name, err)
		}
		existingProtocol, existingPort, ok := proxyRemotePortValue(existingCfg)
		if ok && existingProtocol == protocol && existingPort == port {
			return false, fmt.Errorf("%s proxy remote port %d is already assigned to proxy %q", protocol, port, existing.Name)
		}
	}
	if reservationConflict != nil && !configuredProxyOwnsPort {
		return false, fmt.Errorf("%s proxy remote port %d is already reserved by %s %s", protocol, port, reservationConflict.ResourceType, reservationConflict.ResourceID)
	}
	if !reservationOwnsPort && !configuredProxyOwnsPort {
		if conflict, err := findObservedProxyPortConflict(query, userInfo, serverID, protocol, port); err != nil {
			return false, err
		} else if conflict != "" {
			return false, fmt.Errorf("%s proxy remote port %d is currently used by %s", protocol, port, conflict)
		}
	}

	// Socket snapshots and configured proxy rows are observations/checks. The
	// reservation is the serialized intent that closes the race between two
	// concurrent tunnel writes. Keep the identity stable across shadow clients
	// so editing an existing tunnel refreshes its own reservation.
	created, err := dao.NewMutation(c).ReservePort(userInfo, serverID, defs.CliTypeServer, protocol, port, models.PortReservationTypeProxy, resourceID, reservationAliases...)
	if err != nil {
		return false, fmt.Errorf("reserve %s proxy remote port %d: %w", protocol, port, err)
	}

	return created, nil
}

func proxyRemotePort(cfg v1.TypedProxyConfig) (string, uint16, error) {
	if cfg.ProxyConfigurer == nil {
		return "", 0, fmt.Errorf("proxy configuration is required")
	}
	protocol, port, ok := proxyRemotePortValue(cfg)
	if !ok {
		if cfg.GetBaseConfig() == nil {
			return "", 0, fmt.Errorf("proxy base configuration is required")
		}
		switch cfg.GetBaseConfig().Type {
		case string(v1.ProxyTypeTCP), string(v1.ProxyTypeUDP):
			return cfg.GetBaseConfig().Type, 0, fmt.Errorf("%s proxy remotePort must be between 1 and 65535", cfg.GetBaseConfig().Type)
		default:
			return "", 0, nil
		}
	}
	return protocol, port, nil
}

func proxyRemotePortValue(cfg v1.TypedProxyConfig) (string, uint16, bool) {
	if cfg.ProxyConfigurer == nil || cfg.GetBaseConfig() == nil {
		return "", 0, false
	}
	protocol := cfg.GetBaseConfig().Type
	var remotePort int
	switch typed := cfg.ProxyConfigurer.(type) {
	case *v1.TCPProxyConfig:
		remotePort = typed.RemotePort
		protocol = string(v1.ProxyTypeTCP)
	case *v1.UDPProxyConfig:
		remotePort = typed.RemotePort
		protocol = string(v1.ProxyTypeUDP)
	default:
		return "", 0, false
	}
	if remotePort <= 0 || remotePort > 65535 {
		return protocol, 0, false
	}
	return protocol, uint16(remotePort), true
}

func proxyReservedServerPort(protocol string, port uint16, config *v1.ServerConfig) string {
	return models.ServerPortReservationConflict(config, protocol, port)
}

func findObservedProxyPortConflict(
	query dao.PortSnapshotQuery,
	userInfo models.UserInfo,
	serverID, protocol string,
	port uint16,
) (string, error) {
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
	for _, socket := range sockets {
		if socket == nil || socket.Port != port || !strings.EqualFold(socket.Protocol, protocol) {
			continue
		}
		process := strings.TrimSpace(socket.Process)
		if process == "" {
			process = "an existing process"
		}
		observed = append(observed, fmt.Sprintf("%s %s:%d", process, socket.Address, socket.Port))
	}
	return strings.Join(observed, ", "), nil
}

func sameProxyClient(existing *models.ProxyConfig, clientID, originClientID string) bool {
	if existing == nil {
		return false
	}
	if clientID != "" && (existing.ClientID == clientID || existing.OriginClientID == clientID) {
		return true
	}
	return originClientID != "" && (existing.ClientID == originClientID || existing.OriginClientID == originClientID)
}

func proxyReservationResourceID(serverID, clientID, originClientID, proxyName string) string {
	owner := strings.TrimSpace(originClientID)
	if owner == "" {
		owner = strings.TrimSpace(clientID)
	}
	return fmt.Sprintf("%s/%s/%s", strings.TrimSpace(serverID), owner, strings.TrimSpace(proxyName))
}
