package rank

import (
	"gameserver/common/event_dispatcher"
	"gameserver/core/chanrpc"
	"gameserver/core/module"
	"gameserver/modules/rank/internal"
	"gameserver/modules/rank/internal/managers"
)

type RankExternal struct {
	RankManager *managers.RankManager
	Module      *internal.Module
	ChanRPC     *chanrpc.Server
}

var External = &RankExternal{}

func (m *RankExternal) InitExternal() {
	m.Module = new(internal.Module)
	m.ChanRPC = internal.ChanRPC
	m.RankManager = managers.GetRankManager()
	event_dispatcher.RegisterDispatcher(m.ChanRPC)
}

func (m *RankExternal) GetModule() module.Module {
	return m.Module
}
