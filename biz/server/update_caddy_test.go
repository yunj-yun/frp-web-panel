package server

import (
	"errors"
	"strings"
	"testing"
)

func TestCaddyApplyErrorMessageExplainsUnreachableAdminAPI(t *testing.T) {
	err := errors.New(`adapt Caddyfile: Post "http://127.0.0.1:2019/adapt?adapter=caddyfile": dial tcp 127.0.0.1:2019: connect: connection refused`)
	message := caddyApplyErrorMessage(err)
	if !strings.Contains(message, "connection refused") {
		t.Fatalf("original error was dropped: %q", message)
	}
	// The hint must point at the container state, not ACME or DNS, because a
	// stopped Caddy container is the usual cause of this failure.
	if !strings.Contains(message, "docker compose ps") {
		t.Fatalf("message does not point at the container state: %q", message)
	}
}

func TestCaddyApplyErrorMessagePassesOtherErrorsThrough(t *testing.T) {
	err := errors.New("load Caddy configuration: Caddy admin API 400: invalid Caddyfile")
	if got := caddyApplyErrorMessage(err); got != err.Error() {
		t.Fatalf("unrelated error was rewritten: %q", got)
	}
	if got := caddyApplyErrorMessage(nil); got != "" {
		t.Fatalf("nil error produced %q", got)
	}
}
