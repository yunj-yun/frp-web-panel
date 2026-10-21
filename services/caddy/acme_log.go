package caddy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yunj-yun/frp-web-panel/pb"
)

// maxACMETailBytes bounds how much of the Caddy log is read on each status
// refresh. Caddy appends structured JSON lines, so only the tail matters for
// surfacing the most recent ACME failure.
const maxACMETailBytes = 4 * 1024 * 1024

// maxACMEErrorMessage caps the message stored in the database and rendered in
// the UI. ACME errors embed the full challenge URL and upstream detail.
const maxACMEErrorMessage = 2048

// acmeLogRecord is the subset of Caddy's structured log line used to detect a
// failed certificate order. Caddy logs ACME work under the `tls.obtain` and
// `http.acme_client` loggers, so both are considered.
type acmeLogRecord struct {
	Level      string          `json:"level"`
	TS         float64         `json:"ts"`
	Logger     string          `json:"logger"`
	Msg        string          `json:"msg"`
	Identifier string          `json:"identifier"`
	Error      string          `json:"error"`
	Problem    json.RawMessage `json:"problem"`
}

type acmeProblem struct {
	Type   string `json:"type"`
	Detail string `json:"detail"`
}

// ACMEFailure is the most recent issuance failure recorded by Caddy for one
// domain. It is deliberately provider agnostic: the text comes straight from
// Caddy so DNS, firewall, and rate-limit causes all reach the management UI.
type ACMEFailure struct {
	Domain     string
	Message    string
	OccurredAt time.Time
}

// ScanACMEFailures reads Caddy's JSON log and returns the latest issuance
// failure per domain. A missing or unreadable log is not an error: fresh
// installations simply have nothing to report yet.
func ScanACMEFailures(dataDir string) (map[string]*ACMEFailure, error) {
	file, err := os.Open(ACMELogPath(dataDir))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]*ACMEFailure{}, nil
		}
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReaderSize(file, 64*1024)
	if info, statErr := file.Stat(); statErr == nil && info.Size() > maxACMETailBytes {
		if _, seekErr := file.Seek(-maxACMETailBytes, io.SeekEnd); seekErr != nil {
			return nil, seekErr
		}
		// A tail window usually starts mid-line; drop the truncated first line.
		if _, readErr := reader.ReadString('\n'); readErr != nil && readErr != io.EOF {
			return nil, readErr
		}
	}
	return parseACMEFailures(reader)
}

// ACMELogPath returns the log file written by the generated Caddyfile. Caddy
// runs with its data directory mounted at /data in the default Compose layout.
func ACMELogPath(dataDir string) string {
	return filepath.Join(caddyDataDir(dataDir), "logs", "caddy.log")
}

// caddyDataDir normalizes the configured Caddy data directory. An unset value
// must resolve to the Compose default rather than a relative path, otherwise
// the Agent would look for the log next to its working directory.
func caddyDataDir(dataDir string) string {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return "/data"
	}
	return filepath.Clean(dataDir)
}

func parseACMEFailures(reader *bufio.Reader) (map[string]*ACMEFailure, error) {
	failures := make(map[string]*ACMEFailure)
	for {
		line, err := reader.ReadString('\n')
		if len(strings.TrimSpace(line)) > 0 {
			record := acmeLogRecord{}
			if jsonErr := json.Unmarshal([]byte(line), &record); jsonErr == nil {
				if failure := acmeFailureFromRecord(&record); failure != nil {
					existing := failures[failure.Domain]
					// Keep the newest failure per domain; a tail window may include a
					// retry line after the original cause.
					if existing == nil || !failure.OccurredAt.Before(existing.OccurredAt) {
						failures[failure.Domain] = failure
					}
				}
			}
		}
		if err == io.EOF {
			return failures, nil
		}
		if err != nil {
			return failures, err
		}
	}
}

// acmeFailureFromRecord extracts a human readable ACME failure. Only lines
// that carry a domain and describe a real order failure are considered, so
// transient retry notices do not overwrite the underlying cause.
func acmeFailureFromRecord(record *acmeLogRecord) *ACMEFailure {
	if record == nil || strings.ToLower(record.Level) != "error" {
		return nil
	}
	domain := normalizeDomain(record.Identifier)
	if domain == "" {
		return nil
	}
	message := acmeFailureMessage(record)
	if message == "" {
		return nil
	}
	occurredAt := time.Time{}
	if record.TS > 0 {
		occurredAt = time.Unix(0, int64(record.TS*float64(time.Second)))
	}
	return &ACMEFailure{Domain: domain, Message: message, OccurredAt: occurredAt}
}

func acmeFailureMessage(record *acmeLogRecord) string {
	detail := ""
	if len(record.Problem) > 0 {
		problem := acmeProblem{}
		if json.Unmarshal(record.Problem, &problem) == nil {
			detail = strings.TrimSpace(problem.Detail)
			if detail == "" {
				detail = strings.TrimSpace(problem.Type)
			}
		}
	}
	if detail == "" {
		detail = strings.TrimSpace(record.Error)
	}
	if detail == "" {
		return ""
	}
	prefix := strings.TrimSpace(record.Msg)
	if prefix != "" && !strings.Contains(detail, prefix) {
		detail = fmt.Sprintf("%s: %s", prefix, detail)
	}
	if len(detail) > maxACMEErrorMessage {
		detail = detail[:maxACMEErrorMessage] + "..."
	}
	return detail
}

// MergeACMEFailures combines certificate metadata with the latest issuance
// failures. A failed order usually leaves no certificate file behind, so a
// failure without a matching certificate is reported as an error entry; this
// is what lets the management UI explain why a certificate stayed pending.
//
// A certificate that is already issued wins over the log: the log may still
// contain an older failure from a previous attempt, and reporting that on a
// working certificate would be misleading. Existing Agent-provided messages
// are preserved for the same reason.
func MergeACMEFailures(items []*pb.CaddyCertificateStatus, failures map[string]*ACMEFailure) []*pb.CaddyCertificateStatus {
	if len(failures) == 0 {
		return items
	}
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		domain := normalizeDomain(item.GetDomain())
		if domain == "" {
			continue
		}
		seen[domain] = struct{}{}
		if strings.TrimSpace(item.GetErrorMessage()) != "" {
			continue
		}
		// Only pending/expired/error entries describe an unissued certificate.
		// A ready certificate must not inherit a historical failure.
		switch strings.ToLower(strings.TrimSpace(item.GetStatus())) {
		case "", "pending", "renewing", "expired", "error":
		default:
			continue
		}
		if failure := failures[domain]; failure != nil {
			item.ErrorMessage = failure.Message
		}
	}
	merged := items
	for domain, failure := range failures {
		if _, ok := seen[domain]; ok {
			continue
		}
		merged = append(merged, &pb.CaddyCertificateStatus{
			Domain:       domain,
			Status:       "error",
			ErrorMessage: failure.Message,
		})
	}
	return merged
}
