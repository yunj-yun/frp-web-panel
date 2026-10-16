package shell

import (
	"github.com/yunj-yun/frp-web-panel/pb"
	"github.com/yunj-yun/frp-web-panel/services/app"
	"github.com/yunj-yun/frp-web-panel/utils"
)

type PTYMgr struct {
	*utils.SyncMap[string, pb.Master_PTYConnectServer]                                   // sessionID
	doneMap                                            *utils.SyncMap[string, chan bool] // sessionID
}

func (m *PTYMgr) IsSessionDone(sessionID string) bool {
	ch, ok := m.doneMap.Load(sessionID)
	if !ok {
		return true
	}
	return <-ch
}

func (m *PTYMgr) SetSessionDone(sessionID string) {
	ch, ok := m.doneMap.Load(sessionID)
	if !ok {
		return
	}
	ch <- true
}

func (m *PTYMgr) Add(sessionID string, conn pb.Master_PTYConnectServer) {
	m.Store(sessionID, conn)
	m.doneMap.Store(sessionID, make(chan bool))
}

func NewPTYMgr() app.ShellPTYMgr {
	return &PTYMgr{
		SyncMap: &utils.SyncMap[string, pb.Master_PTYConnectServer]{},
		doneMap: &utils.SyncMap[string, chan bool]{},
	}
}
