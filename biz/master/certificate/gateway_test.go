package certificate

import (
	"strings"
	"testing"

	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/yunj-yun/frp-web-panel/models"
)

func TestNormalizeGatewayDomain(t *testing.T) {
	domain, err := normalizeGatewayDomain("  API.Example.com. ")
	if err != nil {
		t.Fatalf("normalize gateway domain: %v", err)
	}
	if domain != "api.example.com" {
		t.Fatalf("normalized domain = %q", domain)
	}

	for _, input := range []string{"", "localhost", "127.0.0.1", "*.example.com", "-invalid.example.com"} {
		if _, err := normalizeGatewayDomain(input); err == nil {
			t.Fatalf("normalizeGatewayDomain(%q) accepted an invalid domain", input)
		}
	}
	if _, err := normalizeGatewayDomain("a." + strings.Repeat("b", 250)); err == nil {
		t.Fatal("normalizeGatewayDomain accepted a domain longer than 253 bytes")
	}
}

func TestNormalizeGatewayPort(t *testing.T) {
	if port, err := normalizeGatewayPort(0); err != nil || port != 443 {
		t.Fatalf("default gateway port = %d, err = %v", port, err)
	}
	if port, err := normalizeGatewayPort(65535); err != nil || port != 65535 {
		t.Fatalf("maximum gateway port = %d, err = %v", port, err)
	}
	if _, err := normalizeGatewayPort(65536); err == nil {
		t.Fatal("normalizeGatewayPort accepted an out-of-range port")
	}
}

func TestCertificateCoversDomain(t *testing.T) {
	certificate := &models.ManagedCertificate{Domains: models.GormArray[string]{"example.com", "*.example.net"}}
	for _, test := range []struct {
		domain string
		want   bool
	}{
		{domain: "example.com", want: true},
		{domain: "api.example.net", want: true},
		{domain: "deep.api.example.net", want: false},
		{domain: "other.example.org", want: false},
	} {
		if got := certificateCoversDomain(certificate, test.domain); got != test.want {
			t.Errorf("certificateCoversDomain(%q) = %v, want %v", test.domain, got, test.want)
		}
	}
}

func TestCertificateCoversPrimaryDomain(t *testing.T) {
	certificate := &models.ManagedCertificate{PrimaryDomain: "API.Example.com"}
	if !certificateCoversDomain(certificate, "api.example.com") {
		t.Fatal("certificate primary domain was not considered")
	}
}

func TestContainsGatewayDomain(t *testing.T) {
	if !containsGatewayDomain([]string{"WWW.Example.com.", "api.example.com"}, "www.example.com") {
		t.Fatal("containsGatewayDomain did not normalize a custom domain")
	}
	if containsGatewayDomain([]string{"api.example.com"}, "www.example.com") {
		t.Fatal("containsGatewayDomain matched an unrelated domain")
	}
	if !containsGatewaySubdomain("app", "Example.com.", "app.example.com") {
		t.Fatal("containsGatewaySubdomain did not resolve a valid FRP subdomain")
	}
	if containsGatewaySubdomain("nested.app", "example.com", "nested.app.example.com") {
		t.Fatal("containsGatewaySubdomain accepted a multi-label subdomain")
	}
}

func TestServerPortReservationConflictBlocksVhostCandidates(t *testing.T) {
	// The assignment loop relies on this helper to skip ports frps already owns.
	cfg := &v1.ServerConfig{BindPort: 7000, WebServer: v1.WebServerConfig{Port: 7500}}
	for _, tc := range []struct {
		port uint16
		want bool
	}{
		{port: 7000, want: true},  // bindPort
		{port: 7500, want: true},  // webServer.port
		{port: 8080, want: false}, // free
		{port: 443, want: true},   // Caddy owns it
		{port: 80, want: true},    // Caddy owns it
	} {
		got := models.ServerPortReservationConflict(cfg, "tcp", tc.port) != ""
		if got != tc.want {
			t.Errorf("port %d conflict = %v, want %v", tc.port, got, tc.want)
		}
	}
}
