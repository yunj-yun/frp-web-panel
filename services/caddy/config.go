// Package caddy contains the small, deterministic part of the Caddy control
// plane.  It deliberately does not talk to the Caddy admin API: the Master
// renders a complete desired configuration and the Server Agent applies it
// atomically on the target host.
package caddy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/yunj-yun/frp-web-panel/models"
)

var domainPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)

// RenderGatewayCaddyfile renders the complete desired Caddyfile for a Server.
// A route is rendered as a TLS virtual host and forwards to frps' HTTP vhost
// port.  The original request Host is explicitly retained so frps can select
// the matching HTTP proxy.  Caddy owns ACME issuance and renewal on the
// target server; private keys never enter the Master database.
func RenderGatewayCaddyfile(routes []*models.GatewayRoute) ([]byte, error) {
	return RenderGatewayCaddyfileWithCertificates(routes, nil)
}

// RenderGatewayCaddyfileWithCertificates also emits a harmless certificate
// site for a managed certificate which is not bound to a gateway route yet.
// Caddy can complete ACME issuance as soon as the DNS record is in place; a
// later route sync replaces the response-only site with the FRP upstream.
func RenderGatewayCaddyfileWithCertificates(routes []*models.GatewayRoute, certificates []*models.ManagedCertificate) ([]byte, error) {
	items := make([]models.GatewayRoute, 0, len(routes))
	seen := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		if route == nil {
			continue
		}
		copy := *route
		copy.Domain = normalizeDomain(copy.Domain)
		if err := validateRoute(&copy); err != nil {
			return nil, err
		}
		key := copy.Domain + ":" + strconv.Itoa(int(copy.ListenPort))
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate caddy route %s", key)
		}
		seen[key] = struct{}{}
		items = append(items, copy)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Domain != items[j].Domain {
			return items[i].Domain < items[j].Domain
		}
		return items[i].ListenPort < items[j].ListenPort
	})
	certDomains := make([]string, 0, len(certificates))
	certificateChallenges := make(map[uint]string, len(certificates))
	domainChallenges := make(map[string]string)
	for _, certificate := range certificates {
		if certificate == nil {
			continue
		}
		for _, rawDomain := range certificate.Domains {
			domain := normalizeDomain(rawDomain)
			if domain == "" {
				continue
			}
			if net.ParseIP(domain) != nil || !domainPattern.MatchString(domain) {
				return nil, fmt.Errorf("invalid managed certificate domain %q", rawDomain)
			}
			certDomains = append(certDomains, domain)
			if challenge := normalizeChallenge(certificate.Challenge); challenge != "" {
				domainChallenges[domain] = challenge
			}
		}
		certificateChallenges[certificate.ID] = normalizeChallenge(certificate.Challenge)
	}
	sort.Strings(certDomains)
	certDomains = uniqueStrings(certDomains)

	var b strings.Builder
	// Keep Caddy's automatic HTTPS challenge listener enabled while avoiding an
	// implicit HTTP-to-HTTPS redirect for FRP routes that may also be served by
	// an explicitly configured HTTP proxy.
	//
	// Caddy binds port 80 only for the duration of an HTTP-01 challenge, so no
	// explicit :80 site is required. The global log directive persists ACME
	// diagnostics next to the certificate data, which lets the Server Agent
	// report why an issuance failed instead of leaving it pending forever.
	b.WriteString("{\n\tauto_https disable_redirects\n")
	b.WriteString("\tlog {\n")
	b.WriteString("\t\toutput file {$CADDY_DATA_DIR:/data}/logs/caddy.log {\n")
	b.WriteString("\t\t\troll_size 10MiB\n")
	b.WriteString("\t\t\troll_keep 3\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t\tlevel INFO\n")
	b.WriteString("\t}\n")
	b.WriteString("}\n")
	for _, route := range items {
		renderRouteSite(&b, &route, certificateChallenges[route.CertificateID])
	}
	for _, domain := range certDomains {
		// A route may already own the canonical :443 site.  Do not emit a
		// duplicate site block; Caddy rejects duplicate addresses.
		if _, ok := seen[domain+":443"]; ok {
			continue
		}
		fmt.Fprintf(&b, "\n%s:443 {\n", domain)
		renderTLS(&b, domainChallenges[domain])
		b.WriteString("\trespond \"managed certificate is ready\" 404\n}\n")
	}
	return []byte(b.String()), nil
}

func renderRouteSite(b *strings.Builder, route *models.GatewayRoute, challenge string) {
	fmt.Fprintf(b, "\n%s:%d {\n", route.Domain, route.ListenPort)
	renderTLS(b, challenge)
	fmt.Fprintf(b, "\treverse_proxy 127.0.0.1:%d {\n", route.UpstreamPort)
	b.WriteString("\t\theader_up Host {http.request.host}\n")
	b.WriteString("\t\theader_up X-Forwarded-Proto {http.request.proto}\n")
	b.WriteString("\t}\n")
	b.WriteString("}\n")
}

// renderTLS makes the certificate challenge selected in the management page
// explicit.  Caddy automatically enables both ACME challenges by default;
// disabling the other one is the supported way to force a single challenge
// without introducing a DNS provider plugin.
func renderTLS(b *strings.Builder, challenge string) {
	switch normalizeChallenge(challenge) {
	case models.CertificateChallengeHTTP01:
		b.WriteString("\ttls {\n\t\tissuer acme {\n\t\t\tdisable_tlsalpn_challenge\n\t\t}\n\t}\n")
	case models.CertificateChallengeTLSALPN01:
		b.WriteString("\ttls {\n\t\tissuer acme {\n\t\t\tdisable_http_challenge\n\t\t}\n\t}\n")
	default:
		b.WriteString("\ttls\n")
	}
}

func normalizeChallenge(challenge string) string {
	switch strings.ToLower(strings.TrimSpace(challenge)) {
	case models.CertificateChallengeHTTP01:
		return models.CertificateChallengeHTTP01
	case models.CertificateChallengeTLSALPN01:
		return models.CertificateChallengeTLSALPN01
	default:
		return ""
	}
}

func uniqueStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:0]
	for _, value := range values {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

// ConfigHash returns the stable hash sent with an UpdateCaddy command.  It is
// exported so an Agent can make applying the same configuration idempotent.
func ConfigHash(caddyfile []byte) string {
	sum := sha256.Sum256(caddyfile)
	return hex.EncodeToString(sum[:])
}

func normalizeDomain(value string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
}

func validateRoute(route *models.GatewayRoute) error {
	if route == nil {
		return fmt.Errorf("gateway route is required")
	}
	if route.Domain == "" || len(route.Domain) > 253 || net.ParseIP(route.Domain) != nil || !domainPattern.MatchString(route.Domain) {
		return fmt.Errorf("invalid gateway domain %q", route.Domain)
	}
	if route.ListenPort == 0 || route.UpstreamPort == 0 {
		return fmt.Errorf("gateway route ports must be between 1 and 65535")
	}
	if strings.ContainsAny(route.Domain, "{}()[]\\\"'`\n\r\t ") {
		return fmt.Errorf("gateway domain contains unsupported characters")
	}
	return nil
}
