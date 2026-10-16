package ports

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestParseSSOutput(t *testing.T) {
	observedAt := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	output := `tcp LISTEN 0 4096 0.0.0.0:443 0.0.0.0:* users:(("caddy",pid=42,fd=7))
udp UNCONN 0 0 127.0.0.1:5353 0.0.0.0:* users:(("dnsmasq",pid=8,fd=4))
tcp LISTEN 0 128 [::]:7000 [::]:* users:(("frps",pid=99,fd=5))
tcp LISTEN 0 128 0.0.0.0:* 0.0.0.0:*
unix STREAM 0 0 /tmp/example -
`

	snapshots, err := ParseSSOutput(output, observedAt)
	if err != nil {
		t.Fatalf("parse ss output: %v", err)
	}
	if len(snapshots) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(snapshots))
	}
	if got := snapshots[0]; got.Protocol != ProtocolTCP || got.Port != 443 || got.Address != "0.0.0.0" || got.Process != "caddy" || got.PID != 42 {
		t.Fatalf("unexpected first snapshot: %+v", got)
	}
	if snapshots[0].ObservedAt != observedAt {
		t.Fatalf("snapshot time changed: %v", snapshots[0].ObservedAt)
	}
	if got := snapshots[1]; got.Port != 7000 || got.Address != "::" || got.Process != "frps" {
		t.Fatalf("unexpected IPv6 snapshot: %+v", got)
	}
}

func TestScannerScan(t *testing.T) {
	called := false
	scanner := &Scanner{
		run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			called = true
			if name != "ss" || len(args) != 2 || args[0] != "-H" || args[1] != "-ltnup" {
				t.Fatalf("unexpected command: %s %v", name, args)
			}
			return []byte("tcp LISTEN 0 128 127.0.0.1:9000 0.0.0.0:*"), nil
		},
		now: func() time.Time { return time.Unix(10, 0) },
	}

	snapshots, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !called || len(snapshots) != 1 || snapshots[0].Port != 9000 || snapshots[0].ObservedAt.Unix() != 10 {
		t.Fatalf("unexpected scan result: called=%v snapshots=%+v", called, snapshots)
	}
}

func TestScannerScanError(t *testing.T) {
	scanner := &Scanner{run: func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("ss unavailable")
	}}
	if _, err := scanner.Scan(context.Background()); err == nil {
		t.Fatal("expected scanner error")
	}
}

func TestScannerUsesBuiltInFallbackWhenSSUnavailable(t *testing.T) {
	observedAt := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	scanner := &Scanner{
		run: func(context.Context, string, ...string) ([]byte, error) {
			return nil, errors.New("ss unavailable")
		},
		fallback: func(_ context.Context, at time.Time) ([]Snapshot, error) {
			return []Snapshot{{Protocol: ProtocolTCP, Address: "127.0.0.1", Port: 8080, ObservedAt: at}}, nil
		},
		now: func() time.Time { return observedAt },
	}

	snapshots, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("scan with built-in fallback: %v", err)
	}
	if len(snapshots) != 1 || snapshots[0].Port != 8080 || snapshots[0].ObservedAt != observedAt {
		t.Fatalf("unexpected fallback snapshot: %+v", snapshots)
	}
}
