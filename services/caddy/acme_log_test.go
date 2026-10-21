package caddy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yunj-yun/frp-web-panel/pb"
)

func writeACMELog(t *testing.T, dir string, lines []string) {
	t.Helper()
	logDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := ""
	for _, line := range lines {
		content += line + "\n"
	}
	if err := os.WriteFile(filepath.Join(logDir, "caddy.log"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestScanACMEFailuresExtractsLatestFailurePerDomain(t *testing.T) {
	dir := t.TempDir()
	writeACMELog(t, dir, []string{
		`{"level":"info","ts":100,"logger":"tls.obtain","msg":"obtaining certificate","identifier":"app.example.com"}`,
		`{"level":"error","ts":200,"logger":"http.acme_client","msg":"challenge failed","identifier":"app.example.com","challenge_type":"http-01","problem":{"type":"urn:ietf:params:acme:error:connection","detail":"dial tcp 1.2.3.4:80: connect: connection refused"}}`,
		`{"level":"error","ts":300,"logger":"tls.obtain","msg":"could not get certificate from issuer","identifier":"app.example.com","error":"HTTP 400 urn:ietf:params:acme:error:dns"}`,
		`{"level":"error","ts":250,"logger":"tls.obtain","msg":"could not get certificate from issuer","identifier":"api.example.com","error":"HTTP 400 urn:ietf:params:acme:error:rejectedIdentifier"}`,
	})
	failures, err := ScanACMEFailures(dir)
	if err != nil {
		t.Fatalf("scan ACME failures: %v", err)
	}
	if len(failures) != 2 {
		t.Fatalf("expected 2 failed domains, got %d: %+v", len(failures), failures)
	}
	app := failures["app.example.com"]
	if app == nil {
		t.Fatal("app.example.com failure was not reported")
	}
	// The newest record wins so the UI shows the final cause, not an earlier retry.
	if !strings.Contains(app.Message, "urn:ietf:params:acme:error:dns") {
		t.Fatalf("latest failure not selected: %q", app.Message)
	}
	if failures["api.example.com"] == nil {
		t.Fatal("api.example.com failure was not reported")
	}
}

func TestScanACMEFailuresIgnoresNonErrorAndUnrelatedRecords(t *testing.T) {
	dir := t.TempDir()
	writeACMELog(t, dir, []string{
		`{"level":"info","ts":100,"logger":"tls.obtain","msg":"certificate obtained successfully","identifier":"app.example.com"}`,
		`{"level":"warn","ts":150,"logger":"http.acme_client","msg":"HTTP request failed; retrying","url":"https://acme.example/dir"}`,
		`{"level":"error","ts":200,"logger":"http","msg":"server error"}`,
		`not json at all`,
		``,
	})
	failures, err := ScanACMEFailures(dir)
	if err != nil {
		t.Fatalf("scan ACME failures: %v", err)
	}
	if len(failures) != 0 {
		t.Fatalf("expected no ACME failures, got %+v", failures)
	}
}

func TestScanACMEFailuresWithoutLogFileIsNotAnError(t *testing.T) {
	failures, err := ScanACMEFailures(t.TempDir())
	if err != nil {
		t.Fatalf("missing log should not fail: %v", err)
	}
	if len(failures) != 0 {
		t.Fatalf("expected empty result, got %+v", failures)
	}
}

func TestMergeACMEFailuresReportsFailuresWithoutCertificate(t *testing.T) {
	items := []*pb.CaddyCertificateStatus{{Domain: "ready.example.com", Status: "ready"}}
	merged := MergeACMEFailures(items, map[string]*ACMEFailure{
		"pending.example.com": {Domain: "pending.example.com", Message: "challenge failed: connection refused"},
		"ready.example.com":   {Domain: "ready.example.com", Message: "stale failure"},
	})
	byDomain := make(map[string]*pb.CaddyCertificateStatus, len(merged))
	for _, item := range merged {
		byDomain[item.GetDomain()] = item
	}
	// A failed order leaves no certificate file; the error entry is what makes
	// the pending certificate explainable in the management UI.
	failed := byDomain["pending.example.com"]
	if failed == nil || failed.GetStatus() != "error" || failed.GetErrorMessage() == "" {
		t.Fatalf("missing synthetic error entry: %+v", failed)
	}
	// A certificate that was issued successfully must keep its ready status.
	ready := byDomain["ready.example.com"]
	if ready == nil || ready.GetStatus() != "ready" {
		t.Fatalf("ready certificate was changed: %+v", ready)
	}
	if ready.GetErrorMessage() != "" {
		t.Fatalf("issued certificate received an error message: %q", ready.GetErrorMessage())
	}
}

func TestRenderGatewayCaddyfileWritesACMELog(t *testing.T) {
	got, err := RenderGatewayCaddyfile(nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, fragment := range []string{
		"log {",
		"output file {$CADDY_DATA_DIR:/data}/logs/caddy.log",
		"roll_size 10MiB",
		"level INFO",
	} {
		if !strings.Contains(string(got), fragment) {
			t.Errorf("rendered Caddyfile does not contain %q: %s", fragment, got)
		}
	}
}

func TestACMELogPathDefaultsToContainerDataDir(t *testing.T) {
	// An unset CADDY_DATA_DIR must resolve to the Compose mount, not a path
	// relative to the Agent working directory.
	if got, want := ACMELogPath(""), "/data/logs/caddy.log"; got != want {
		t.Fatalf("ACMELogPath(\"\") = %q, want %q", got, want)
	}
	if got, want := ACMELogPath("  "), "/data/logs/caddy.log"; got != want {
		t.Fatalf("ACMELogPath(blank) = %q, want %q", got, want)
	}
	if got, want := ACMELogPath("/data/"), "/data/logs/caddy.log"; got != want {
		t.Fatalf("ACMELogPath(/data/) = %q, want %q", got, want)
	}
}

func TestACMEPathLayoutsStayConsistent(t *testing.T) {
	// The generated Server Compose mounts Caddy's data directory at /caddy-data
	// inside the frp-panel container, while Caddy itself keeps /data. Older
	// deployments use /data for both. The Agent derives both certificate and log
	// locations from the same variable, so every layout must stay self-consistent.
	layouts := []struct {
		dataDir  string
		certRoot string
		logPath  string
	}{
		{dataDir: "/caddy-data", certRoot: "/caddy-data/caddy/certificates", logPath: "/caddy-data/logs/caddy.log"},
		{dataDir: "/data", certRoot: "/data/caddy/certificates", logPath: "/data/logs/caddy.log"},
	}
	for _, layout := range layouts {
		roots := certificateRoots(caddyDataDir(layout.dataDir))
		found := false
		for _, root := range roots {
			if root == layout.certRoot {
				found = true
			}
		}
		if !found {
			t.Errorf("certificate roots for %q do not include %q: %v", layout.dataDir, layout.certRoot, roots)
		}
		if got := ACMELogPath(layout.dataDir); got != layout.logPath {
			t.Errorf("ACMELogPath(%q) = %q, want %q", layout.dataDir, got, layout.logPath)
		}
	}
}
