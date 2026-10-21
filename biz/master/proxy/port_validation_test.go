package proxy

import (
	"testing"

	v1 "github.com/fatedier/frp/pkg/config/v1"
)

func TestProxyRemotePort(t *testing.T) {
	tests := []struct {
		name     string
		cfg      v1.TypedProxyConfig
		protocol string
		port     uint16
		wantErr  bool
	}{
		{
			name: "tcp",
			cfg: v1.TypedProxyConfig{Type: string(v1.ProxyTypeTCP), ProxyConfigurer: &v1.TCPProxyConfig{
				ProxyBaseConfig: v1.ProxyBaseConfig{Type: string(v1.ProxyTypeTCP)}, RemotePort: 8080,
			}},
			protocol: string(v1.ProxyTypeTCP), port: 8080,
		},
		{
			name: "udp",
			cfg: v1.TypedProxyConfig{Type: string(v1.ProxyTypeUDP), ProxyConfigurer: &v1.UDPProxyConfig{
				ProxyBaseConfig: v1.ProxyBaseConfig{Type: string(v1.ProxyTypeUDP)}, RemotePort: 5353,
			}},
			protocol: string(v1.ProxyTypeUDP), port: 5353,
		},
		{
			name: "http has no remote port",
			cfg: v1.TypedProxyConfig{Type: string(v1.ProxyTypeHTTP), ProxyConfigurer: &v1.HTTPProxyConfig{
				ProxyBaseConfig: v1.ProxyBaseConfig{Type: string(v1.ProxyTypeHTTP)},
			}},
		},
		{
			name: "tcp port zero",
			cfg: v1.TypedProxyConfig{Type: string(v1.ProxyTypeTCP), ProxyConfigurer: &v1.TCPProxyConfig{
				ProxyBaseConfig: v1.ProxyBaseConfig{Type: string(v1.ProxyTypeTCP)},
			}},
			wantErr: true,
		},
		{
			name: "udp port too large",
			cfg: v1.TypedProxyConfig{Type: string(v1.ProxyTypeUDP), ProxyConfigurer: &v1.UDPProxyConfig{
				ProxyBaseConfig: v1.ProxyBaseConfig{Type: string(v1.ProxyTypeUDP)}, RemotePort: 65536,
			}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protocol, port, err := proxyRemotePort(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("proxyRemotePort() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if protocol != tt.protocol || port != tt.port {
				t.Fatalf("proxyRemotePort() = %q/%d, want %q/%d", protocol, port, tt.protocol, tt.port)
			}
		})
	}
}

func TestProxyReservedServerPort(t *testing.T) {
	config := &v1.ServerConfig{
		BindPort:      7000,
		VhostHTTPPort: 8080,
		WebServer:     v1.WebServerConfig{Port: 7500},
	}

	if got := proxyReservedServerPort("tcp", 8080, config); got != "frps vhostHTTPPort" {
		t.Fatalf("proxyReservedServerPort() = %q, want vhostHTTPPort", got)
	}
	if got := proxyReservedServerPort("tcp", 9000, config); got != "" {
		t.Fatalf("proxyReservedServerPort() = %q, want empty", got)
	}
}

func TestProxyReservedServerPortWithoutServerConfig(t *testing.T) {
	// A Server that has been created but not configured yet has no frps config.
	// The configured-listener comparison must be skipped instead of failing the
	// whole tunnel creation; Caddy's own listeners stay reserved regardless
	// because they do not depend on the frps config.
	if got := proxyReservedServerPort("tcp", 8080, nil); got != "" {
		t.Fatalf("proxyReservedServerPort(nil) = %q, want empty", got)
	}
	for _, port := range []uint16{80, 443} {
		if got := proxyReservedServerPort("tcp", port, nil); got == "" {
			t.Fatalf("Caddy listener %d must stay reserved even without a server config", port)
		}
	}
}
