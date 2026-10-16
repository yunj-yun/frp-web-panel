package caddy

import (
	"strings"
	"testing"

	"github.com/VaalaCat/frp-panel/models"
)

func TestRenderGatewayCaddyfileDeterministicAndPreservesHost(t *testing.T) {
	routes := []*models.GatewayRoute{
		{Domain: "B.example.com.", ListenPort: 443, UpstreamPort: 8080},
		{Domain: "a.example.com", ListenPort: 443, UpstreamPort: 18080},
	}
	got, err := RenderGatewayCaddyfile(routes)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	wantOrder := []string{"a.example.com:443", "b.example.com:443"}
	last := -1
	for _, marker := range wantOrder {
		at := strings.Index(string(got), marker)
		if at <= last {
			t.Fatalf("route %q is not in deterministic order: %s", marker, got)
		}
		last = at
	}
	for _, fragment := range []string{
		"auto_https disable_redirects",
		"reverse_proxy 127.0.0.1:8080",
		"header_up Host {http.request.host}",
		"header_up X-Forwarded-Proto {http.request.proto}",
	} {
		if !strings.Contains(string(got), fragment) {
			t.Errorf("rendered Caddyfile does not contain %q: %s", fragment, got)
		}
	}
	if ConfigHash(got) != ConfigHash(got) || len(ConfigHash(got)) != 64 {
		t.Fatal("config hash is not stable")
	}
}

func TestRenderGatewayCaddyfileRejectsUnsafeOrDuplicateRoutes(t *testing.T) {
	cases := []struct {
		name   string
		routes []*models.GatewayRoute
	}{
		{name: "duplicate", routes: []*models.GatewayRoute{
			{Domain: "app.example.com", ListenPort: 443, UpstreamPort: 8080},
			{Domain: "APP.EXAMPLE.COM", ListenPort: 443, UpstreamPort: 8081},
		}},
		{name: "unsafe domain", routes: []*models.GatewayRoute{{Domain: "app.example.com {", ListenPort: 443, UpstreamPort: 8080}}},
		{name: "invalid port", routes: []*models.GatewayRoute{{Domain: "app.example.com", ListenPort: 443, UpstreamPort: 0}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := RenderGatewayCaddyfile(tc.routes); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestRenderGatewayCaddyfileEmitsCertificateOnlySite(t *testing.T) {
	got, err := RenderGatewayCaddyfileWithCertificates(nil, []*models.ManagedCertificate{{
		Domains:   []string{"app.example.com", "www.example.com"},
		Challenge: models.CertificateChallengeHTTP01,
	}})
	if err != nil {
		t.Fatalf("render certificate-only config: %v", err)
	}
	for _, fragment := range []string{
		"app.example.com:443 {",
		"www.example.com:443 {",
		"disable_tlsalpn_challenge",
		"respond \"managed certificate is ready\" 404",
	} {
		if !strings.Contains(string(got), fragment) {
			t.Errorf("certificate-only config does not contain %q: %s", fragment, got)
		}
	}
}

func TestRenderGatewayCaddyfileSelectsTLSALPNChallenge(t *testing.T) {
	got, err := RenderGatewayCaddyfileWithCertificates(nil, []*models.ManagedCertificate{{
		Domains:   []string{"app.example.com"},
		Challenge: models.CertificateChallengeTLSALPN01,
	}})
	if err != nil {
		t.Fatalf("render TLS-ALPN config: %v", err)
	}
	if !strings.Contains(string(got), "disable_http_challenge") {
		t.Fatalf("TLS-ALPN config did not disable HTTP challenge: %s", got)
	}
}
