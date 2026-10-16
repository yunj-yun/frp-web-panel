package certificate

import (
	"reflect"
	"testing"

	"github.com/VaalaCat/frp-panel/models"
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
