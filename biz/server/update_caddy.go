package server

import (
	"fmt"
	"os"
	"strings"

	"github.com/yunj-yun/frp-web-panel/pb"
	"github.com/yunj-yun/frp-web-panel/services/app"
	"github.com/yunj-yun/frp-web-panel/services/caddy"
)

// UpdateCaddyHandler validates the configuration hash supplied by Master and
// applies the complete desired Caddyfile through the local admin API. Caddy's
// data/config directories are persisted by the Server Compose file, so ACME
// certificates and private keys survive Agent restarts.
func UpdateCaddyHandler(ctx *app.Context, req *pb.UpdateCaddyRequest) (*pb.UpdateCaddyResponse, error) {
	if req == nil {
		return &pb.UpdateCaddyResponse{Status: &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: "request is required"}}, fmt.Errorf("request is required")
	}
	if req.GetBase() == nil {
		return &pb.UpdateCaddyResponse{Status: &pb.Status{Code: pb.RespCode_RESP_CODE_UNAUTHORIZED, Message: "server credentials are required"}}, fmt.Errorf("server credentials are required")
	}
	if err := validateServer(ctx, req.GetBase()); err != nil {
		return &pb.UpdateCaddyResponse{Status: &pb.Status{Code: pb.RespCode_RESP_CODE_UNAUTHORIZED, Message: err.Error()}}, err
	}
	config := req.GetCaddyfile()
	if len(config) == 0 {
		return &pb.UpdateCaddyResponse{Status: &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: "Caddyfile is required"}}, fmt.Errorf("Caddyfile is required")
	}
	hash := caddy.ConfigHash(config)
	if expected := strings.TrimSpace(req.GetConfigHash()); expected != "" && expected != hash {
		return &pb.UpdateCaddyResponse{Status: &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: "Caddyfile hash mismatch"}, ConfigHash: hash}, fmt.Errorf("Caddyfile hash mismatch")
	}
	adminURL := strings.TrimSpace(os.Getenv("CADDY_ADMIN_URL"))
	admin, err := caddy.NewAdminClient(adminURL, nil)
	if err != nil {
		return &pb.UpdateCaddyResponse{Status: &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: err.Error()}, ConfigHash: hash}, err
	}
	if err := admin.Apply(ctx, config); err != nil {
		// A refused connection to the admin API almost always means the Caddy
		// container is not running, which is a deployment problem rather than a
		// bad Caddyfile. Say so explicitly; the raw dial error alone sends users
		// looking at DNS and ACME instead of `docker compose ps`.
		return &pb.UpdateCaddyResponse{
			Status:     &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: caddyApplyErrorMessage(err)},
			ConfigHash: hash,
		}, err
	}
	// Caddy loads the configuration before ACME has necessarily completed. Read
	// the public certificate metadata after the load so Master can distinguish a
	// successfully applied route from an actually issued certificate. A missing
	// storage directory is normal on a fresh Server and is reported as an empty
	// status list rather than making a valid configuration fail.
	dataDir := strings.TrimSpace(os.Getenv("CADDY_DATA_DIR"))
	certificates, _ := caddy.ScanCertificates(dataDir)
	return &pb.UpdateCaddyResponse{
		Status:       &pb.Status{Code: pb.RespCode_RESP_CODE_SUCCESS, Message: "Caddy configuration applied"},
		ConfigHash:   hash,
		Certificates: certificates,
	}, nil
}

func validateServer(ctx *app.Context, base *pb.ServerBase) error {
	if ctx == nil || ctx.GetApp() == nil || base == nil {
		return fmt.Errorf("server credentials are required")
	}
	cfg := ctx.GetApp().GetConfig()
	if strings.TrimSpace(base.GetServerId()) == "" || strings.TrimSpace(base.GetServerSecret()) == "" {
		return fmt.Errorf("server credentials are required")
	}
	// An Agent does not have the Master database. Its process configuration is
	// the authenticated identity established during RPC registration, so use it
	// to validate commands received over that already-authenticated channel.
	if base.GetServerId() != cfg.Client.ID || base.GetServerSecret() != cfg.Client.Secret {
		return fmt.Errorf("invalid server credentials")
	}
	return nil
}

// caddyApplyErrorMessage turns a raw admin API failure into an actionable
// message. The most common cause is that the Caddy container never started, so
// the hint points at the container state instead of ACME or DNS.
func caddyApplyErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	lower := strings.ToLower(message)
	if strings.Contains(lower, "connection refused") || strings.Contains(lower, "no such host") || strings.Contains(lower, "connect:") {
		return fmt.Sprintf("%s (Caddy admin API is unreachable: verify the caddy container is running with `docker compose ps` and that CADDY_ADMIN_URL matches its admin listener)", message)
	}
	return message
}
