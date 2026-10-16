package platform

import (
	"strings"

	"github.com/yunj-yun/frp-web-panel/common"
	"github.com/yunj-yun/frp-web-panel/defs"
	"github.com/yunj-yun/frp-web-panel/models"
	"github.com/yunj-yun/frp-web-panel/pb"
	"github.com/yunj-yun/frp-web-panel/services/app"
	"github.com/yunj-yun/frp-web-panel/services/dao"
)

func ListNodePortSnapshots(c *app.Context, req *pb.ListNodePortSnapshotsRequest) (*pb.ListNodePortSnapshotsResponse, error) {
	if req == nil || (len(req.GetClientIds()) == 0 && len(req.GetServerIds()) == 0) {
		return &pb.ListNodePortSnapshotsResponse{Status: &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: "at least one node id is required"}}, nil
	}
	if len(req.GetClientIds())+len(req.GetServerIds()) > 100 {
		return &pb.ListNodePortSnapshotsResponse{Status: &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: "at most 100 node ids can be requested"}}, nil
	}
	for _, nodeID := range append(append([]string(nil), req.GetClientIds()...), req.GetServerIds()...) {
		if strings.TrimSpace(nodeID) == "" || len(nodeID) > 128 {
			return &pb.ListNodePortSnapshotsResponse{Status: &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: "node ids must be non-empty and at most 128 characters"}}, nil
		}
	}

	userInfo := common.GetUserInfo(c)
	if !userInfo.Valid() {
		return &pb.ListNodePortSnapshotsResponse{Status: &pb.Status{Code: pb.RespCode_RESP_CODE_UNAUTHORIZED, Message: "user is not authorized"}}, nil
	}

	query := dao.NewQuery(c)
	scans, err := query.GetNodePortScans(userInfo, req.GetClientIds(), req.GetServerIds())
	if err != nil {
		return nil, err
	}
	scanIDs := make([]uint, 0, len(scans))
	for _, scan := range scans {
		scanIDs = append(scanIDs, scan.ID)
	}
	sockets, err := query.GetNodePortSockets(scanIDs)
	if err != nil {
		return nil, err
	}
	socketMap := make(map[uint][]*models.NodePortSnapshot, len(scans))
	for _, socket := range sockets {
		socketMap[socket.ScanID] = append(socketMap[socket.ScanID], socket)
	}

	result := make([]*pb.NodePortSnapshot, 0, len(scans))
	for _, scan := range scans {
		nodeType := pb.ClientType_CLIENT_TYPE_UNSPECIFIED
		if scan.NodeType == defs.CliTypeClient {
			nodeType = pb.ClientType_CLIENT_TYPE_FRPC
		} else if scan.NodeType == defs.CliTypeServer {
			nodeType = pb.ClientType_CLIENT_TYPE_FRPS
		}
		pbSockets := make([]*pb.PortSocket, 0, len(socketMap[scan.ID]))
		for _, socket := range socketMap[scan.ID] {
			item := &pb.PortSocket{Protocol: socket.Protocol, Address: socket.Address, Port: uint32(socket.Port)}
			if socket.Process != "" {
				item.Process = &socket.Process
			}
			if socket.PID > 0 {
				pid := uint32(socket.PID)
				item.Pid = &pid
			}
			pbSockets = append(pbSockets, item)
		}
		result = append(result, &pb.NodePortSnapshot{
			NodeId:     scan.NodeID,
			NodeType:   nodeType,
			ObservedAt: scan.ObservedAt.UnixMilli(),
			Sockets:    pbSockets,
		})
	}

	return &pb.ListNodePortSnapshotsResponse{Status: &pb.Status{Code: pb.RespCode_RESP_CODE_SUCCESS, Message: "ok"}, Snapshots: result}, nil
}
