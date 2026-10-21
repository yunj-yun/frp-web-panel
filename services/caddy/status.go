package caddy

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yunj-yun/frp-web-panel/pb"
)

const maxCertificateFileSize = 2 * 1024 * 1024

// ScanCertificates reads the public certificate files Caddy stores on the
// target Server. Caddy keeps private keys beside these files, but this
// scanner only reads PEM certificates and returns metadata to Master. The
// default Docker Compose layout mounts Caddy's data directory at /data.
func ScanCertificates(dataDir string) ([]*pb.CaddyCertificateStatus, error) {
	roots := certificateRoots(caddyDataDir(dataDir))
	seenRoots := make(map[string]struct{}, len(roots))
	items := make(map[string]*pb.CaddyCertificateStatus)
	var firstErr error
	for _, root := range roots {
		root = filepath.Clean(root)
		if _, ok := seenRoots[root]; ok {
			continue
		}
		seenRoots[root] = struct{}{}
		if _, err := os.Stat(root); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				if firstErr == nil {
					firstErr = walkErr
				}
				return nil
			}
			if entry.IsDir() {
				return nil
			}
			if !strings.EqualFold(filepath.Ext(entry.Name()), ".crt") {
				return nil
			}
			if err := scanCertificateFile(path, items); err != nil && firstErr == nil {
				firstErr = err
			}
			return nil
		})
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	result := make([]*pb.CaddyCertificateStatus, 0, len(items))
	for _, item := range items {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].GetDomain() < result[j].GetDomain() })
	return result, firstErr
}

func certificateRoots(dataDir string) []string {
	clean := filepath.Clean(dataDir)
	if strings.EqualFold(filepath.Base(clean), "certificates") {
		return []string{clean}
	}
	return []string{
		filepath.Join(clean, "caddy", "certificates"),
		filepath.Join(clean, "certificates"),
	}
}

func scanCertificateFile(path string, items map[string]*pb.CaddyCertificateStatus) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() <= 0 || info.Size() > maxCertificateFileSize {
		return fmt.Errorf("Caddy certificate %s has an invalid size", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" {
		return fmt.Errorf("Caddy certificate %s is not PEM encoded", path)
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse Caddy certificate %s: %w", path, err)
	}
	domains := append([]string(nil), certificate.DNSNames...)
	if len(domains) == 0 {
		domains = []string{strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))}
	}
	fingerprint := sha256.Sum256(certificate.Raw)
	status := "ready"
	if !certificate.NotAfter.After(time.Now()) {
		status = "expired"
	}
	storageID, _ := filepath.Rel(filepath.Dir(filepath.Dir(filepath.Dir(path))), path)
	for _, rawDomain := range domains {
		domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(rawDomain), "."))
		if domain == "" {
			continue
		}
		items[domain] = &pb.CaddyCertificateStatus{
			Domain:      domain,
			Issuer:      certificate.Issuer.String(),
			Fingerprint: hex.EncodeToString(fingerprint[:]),
			StorageId:   storageID,
			IssuedAt:    certificate.NotBefore.UnixMilli(),
			ExpiresAt:   certificate.NotAfter.UnixMilli(),
			Status:      status,
		}
	}
	return nil
}
