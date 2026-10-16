package defs

import "github.com/yunj-yun/frp-web-panel/pb"

type Connector struct {
	CliID   string
	Conn    pb.Master_ServerSendServer
	CliType string
}
