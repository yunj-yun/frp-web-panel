package caddy

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanCertificatesReturnsMetadataWithoutPrivateKey(t *testing.T) {
	root := t.TempDir()
	certDir := filepath.Join(root, "caddy", "certificates", "acme", "example.com")
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	certificateDER, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "example.com"}, DNSNames: []string{"Example.com", "www.example.com"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
	}, &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Example CA"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(48 * time.Hour)}, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(certDir, "example.com.crt")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	items, err := ScanCertificates(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].GetDomain() != "example.com" || items[1].GetDomain() != "www.example.com" {
		t.Fatalf("unexpected certificate metadata: %+v", items)
	}
	if items[0].GetFingerprint() == "" || items[0].GetExpiresAt() <= 0 || items[0].GetStatus() != "ready" {
		t.Fatalf("incomplete certificate metadata: %+v", items[0])
	}
}
