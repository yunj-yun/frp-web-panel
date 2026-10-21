package certificate

import (
	"reflect"
	"testing"
	"time"

	"github.com/yunj-yun/frp-web-panel/models"
)

func TestNormalizeDomains(t *testing.T) {
	domains, primary, err := normalizeDomains([]string{" API.Example.com. ", "www.example.com", "api.example.com"})
	if err != nil {
		t.Fatalf("normalize domains: %v", err)
	}
	if primary != "api.example.com" {
		t.Fatalf("primary domain = %q", primary)
	}
	if !reflect.DeepEqual([]string(domains), []string{"api.example.com", "www.example.com"}) {
		t.Fatalf("domains = %#v", domains)
	}
}

func TestNormalizeDomainsRejectsWildcardAndIP(t *testing.T) {
	for _, input := range [][]string{{"*.example.com"}, {"127.0.0.1"}, {"localhost"}} {
		if _, _, err := normalizeDomains(input); err == nil {
			t.Fatalf("normalizeDomains(%v) accepted an invalid domain", input)
		}
	}
}

func TestNormalizeChallengeDefaultsToHTTP01(t *testing.T) {
	challenge, err := normalizeChallenge("")
	if err != nil || challenge != models.CertificateChallengeHTTP01 {
		t.Fatalf("challenge = %q, err = %v", challenge, err)
	}
	if _, err := normalizeChallenge("dns-01"); err == nil {
		t.Fatal("unsupported challenge was accepted")
	}
}

func TestIsCertificateStaleOnlyFlagsOldPendingRecords(t *testing.T) {
	now := time.Now()
	timeout := 15 * time.Minute

	cases := []struct {
		name      string
		status    string
		updatedAt time.Time
		wantStale bool
	}{
		{name: "old pending", status: models.ManagedCertificateStatusPending, updatedAt: now.Add(-30 * time.Minute), wantStale: true},
		{name: "fresh pending", status: models.ManagedCertificateStatusPending, updatedAt: now.Add(-1 * time.Minute), wantStale: false},
		{name: "old but ready", status: models.ManagedCertificateStatusReady, updatedAt: now.Add(-2 * time.Hour), wantStale: false},
		{name: "old but error", status: models.ManagedCertificateStatusError, updatedAt: now.Add(-2 * time.Hour), wantStale: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			certificate := &models.ManagedCertificate{Status: tc.status}
			certificate.UpdatedAt = tc.updatedAt
			if got := isCertificateStale(certificate, now, timeout); got != tc.wantStale {
				t.Fatalf("isCertificateStale() = %v, want %v", got, tc.wantStale)
			}
		})
	}
	if isCertificateStale(nil, now, timeout) {
		t.Fatal("nil certificate must not be stale")
	}
}
