// Package ports provides host port discovery primitives used by Linux agents.
package ports

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yunj-yun/frp-web-panel/defs"
	gopsutilnet "github.com/shirou/gopsutil/v4/net"
)

const (
	ProtocolTCP = "tcp"
	ProtocolUDP = "udp"
)

// Snapshot is a point-in-time observation of a local listening socket.
type Snapshot struct {
	Protocol   string
	Address    string
	Port       uint16
	Process    string
	PID        int
	ObservedAt time.Time
}

type commandRunner func(context.Context, string, ...string) ([]byte, error)
type snapshotFallback func(context.Context, time.Time) ([]Snapshot, error)

// Scanner discovers listening TCP and UDP sockets from the host namespace.
// It uses ss when available and falls back to gopsutil for minimal images.
type Scanner struct {
	run      commandRunner
	fallback snapshotFallback
	now      func() time.Time
}

func NewScanner() *Scanner {
	return &Scanner{
		run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).Output()
		},
		fallback: scanConnections,
		now:      time.Now,
	}
}

func (s *Scanner) Scan(ctx context.Context) ([]Snapshot, error) {
	if s == nil || s.run == nil {
		return nil, fmt.Errorf("port scanner is not initialized")
	}

	scanCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	observedAt := now()

	output, err := s.run(scanCtx, "ss", "-H", "-ltnup")
	if err == nil {
		return ParseSSOutput(string(output), observedAt)
	}

	if s.fallback == nil {
		return nil, fmt.Errorf("scan listening ports with ss: %w", err)
	}
	snapshots, fallbackErr := s.fallback(scanCtx, observedAt)
	if fallbackErr != nil {
		return nil, fmt.Errorf("scan listening ports with ss: %v; built-in scan: %w", err, fallbackErr)
	}
	return snapshots, nil
}

func scanConnections(ctx context.Context, observedAt time.Time) ([]Snapshot, error) {
	connections, err := gopsutilnet.ConnectionsWithContext(ctx, "inet")
	if err != nil {
		return nil, fmt.Errorf("read internet socket table: %w", err)
	}

	snapshots := make(map[string]Snapshot, len(connections))
	for _, connection := range connections {
		var protocol string
		switch connection.Type {
		case 1:
			if connection.Status != "LISTEN" {
				continue
			}
			protocol = ProtocolTCP
		case 2:
			if connection.Raddr.Port != 0 {
				continue
			}
			protocol = ProtocolUDP
		default:
			continue
		}
		if connection.Laddr.Port == 0 || connection.Laddr.Port > 65535 {
			continue
		}

		address := connection.Laddr.IP
		if address == "" {
			address = "*"
		}
		port := uint16(connection.Laddr.Port)
		key := fmt.Sprintf("%s/%s/%d", protocol, address, port)
		snapshots[key] = Snapshot{
			Protocol:   protocol,
			Address:    address,
			Port:       port,
			PID:        int(connection.Pid),
			ObservedAt: observedAt,
		}
	}
	if len(snapshots) > defs.MaxNodePortSockets {
		return nil, fmt.Errorf("port snapshot exceeds the %d socket limit", defs.MaxNodePortSockets)
	}

	result := make([]Snapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		result = append(result, snapshot)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Protocol != result[j].Protocol {
			return result[i].Protocol < result[j].Protocol
		}
		if result[i].Port != result[j].Port {
			return result[i].Port < result[j].Port
		}
		return result[i].Address < result[j].Address
	})
	return result, nil
}

var processPattern = regexp.MustCompile(`users:\(\("([^"]+)"(?:,pid=([0-9]+))?`)

// ParseSSOutput parses the stable, headerless format produced by `ss -H -ltnup`.
// Rows with an unknown protocol, wildcard port, or malformed local address are
// skipped so one restricted socket cannot hide the rest of the snapshot.
func ParseSSOutput(output string, observedAt time.Time) ([]Snapshot, error) {
	if observedAt.IsZero() {
		observedAt = time.Now()
	}

	result := make([]Snapshot, 0)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		protocol := fields[0]
		if protocol != ProtocolTCP && protocol != ProtocolUDP && protocol != "tcp6" && protocol != "udp6" {
			continue
		}
		if protocol == "tcp6" {
			protocol = ProtocolTCP
		}
		if protocol == "udp6" {
			protocol = ProtocolUDP
		}

		address, port, ok := splitLocalAddress(fields[4])
		if !ok {
			continue
		}

		process, pid := parseProcess(fields[6:])
		result = append(result, Snapshot{
			Protocol:   protocol,
			Address:    address,
			Port:       port,
			Process:    process,
			PID:        pid,
			ObservedAt: observedAt,
		})
		if len(result) > defs.MaxNodePortSockets {
			return nil, fmt.Errorf("port snapshot exceeds the %d socket limit", defs.MaxNodePortSockets)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Protocol != result[j].Protocol {
			return result[i].Protocol < result[j].Protocol
		}
		if result[i].Port != result[j].Port {
			return result[i].Port < result[j].Port
		}
		return result[i].Address < result[j].Address
	})
	return result, nil
}

func splitLocalAddress(raw string) (string, uint16, bool) {
	host, portText, err := net.SplitHostPort(raw)
	if err != nil {
		lastColon := strings.LastIndexByte(raw, ':')
		if lastColon <= 0 || lastColon == len(raw)-1 {
			return "", 0, false
		}
		host, portText = raw[:lastColon], raw[lastColon+1:]
	}
	if host == "" {
		host = "*"
	}
	portNumber, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || portNumber == 0 {
		return "", 0, false
	}
	return host, uint16(portNumber), true
}

func parseProcess(fields []string) (string, int) {
	match := processPattern.FindStringSubmatch(strings.Join(fields, " "))
	if len(match) == 0 {
		return "", 0
	}
	pid, _ := strconv.Atoi(match[2])
	return match[1], pid
}
