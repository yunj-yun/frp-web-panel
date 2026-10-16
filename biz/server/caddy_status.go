package server

import (
	"fmt"
	"os"
	"strings"

	"github.com/VaalaCat/frp-panel/pb"
	"github.com/VaalaCat/frp-panel/services/app"
	"github.com/VaalaCat/frp-panel/services/caddy"
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
	return &pb.GetCaddyStatusResponse{Status: &pb.Status{Code: pb.RespCode_RESP_CODE_SUCCESS, Message: "ok"}, Certificates: certificates}, nil
}
