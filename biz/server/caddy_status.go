package server

import (
	"fmt"
	"os"
	"strings"

	"github.com/yunj-yun/frp-web-panel/pb"
	"github.com/yunj-yun/frp-web-panel/services/app"
	"github.com/yunj-yun/frp-web-panel/services/caddy"
	"github.com/yunj-yun/frp-web-panel/utils/logger"
)

// GetCaddyStatusHandler returns certificate metadata observed in the local
// Caddy storage. It does not reload configuration or expose private keys.
func GetCaddyStatusHandler(ctx *app.Context, req *pb.GetCaddyStatusRequest) (*pb.GetCaddyStatusResponse, error) {
	if req == nil || req.GetBase() == nil {
		return &pb.GetCaddyStatusResponse{Status: &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: "server credentials are required"}}, fmt.Errorf("server credentials are required")
	}
	if err := validateServer(ctx, req.GetBase()); err != nil {
		return &pb.GetCaddyStatusResponse{Status: &pb.Status{Code: pb.RespCode_RESP_CODE_UNAUTHORIZED, Message: err.Error()}}, err
	}
	dataDir := strings.TrimSpace(os.Getenv("CADDY_DATA_DIR"))
	certificates, scanErr := caddy.ScanCertificates(dataDir)
	if scanErr != nil {
		return &pb.GetCaddyStatusResponse{Status: &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: scanErr.Error()}, Certificates: certificates}, scanErr
	}
	certificates = caddy.MergeACMEFailures(certificates, acmeFailures(ctx, dataDir))
	return &pb.GetCaddyStatusResponse{Status: &pb.Status{Code: pb.RespCode_RESP_CODE_SUCCESS, Message: "ok"}, Certificates: certificates}, nil
}

// acmeFailures reads Caddy's ACME diagnostics. A missing or unreadable log is
// reported at debug level and otherwise ignored: certificate metadata read
// from disk remains valid and useful on its own.
func acmeFailures(ctx *app.Context, dataDir string) map[string]*caddy.ACMEFailure {
	failures, err := caddy.ScanACMEFailures(dataDir)
	if err != nil {
		logger.Logger(ctx).WithError(err).Debugf("scan Caddy ACME log")
		return nil
	}
	return failures
}
