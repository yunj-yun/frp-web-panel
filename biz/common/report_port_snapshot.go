package common

import (
	"github.com/VaalaCat/frp-panel/pb"
	"github.com/VaalaCat/frp-panel/services/ports"
)

func PortSockets(sockets []ports.Snapshot) []*pb.PortSocket {
	result := make([]*pb.PortSocket, 0, len(sockets))
	for _, socket := range sockets {
		item := &pb.PortSocket{
			Protocol: socket.Protocol,
			Address:  socket.Address,
			Port:     uint32(socket.Port),
		}
		if socket.Process != "" {
			item.Process = &socket.Process
		}
		if socket.PID > 0 {
			pid := uint32(socket.PID)
			item.Pid = &pid
		}
		result = append(result, item)
	}
	return result
}
