package match

import (
	"gameserver/common/schedule"
	"gameserver/core/chanrpc"
	"gameserver/core/module"
	"gameserver/modules/match/internal"
	"gameserver/modules/match/internal/managers"
)

type MatchExternal struct {
	Module  *internal.Module
	ChanRPC *chanrpc.Server
}

var External = &MatchExternal{}

func (m *MatchExternal) InitExternal() {
	m.Module = new(internal.Module)
	m.ChanRPC = internal.ChanRPC
	schedule.RegisterIntervalSchedul(10, managers.Matching)
}

func (m *MatchExternal) GetModule() module.Module {
	return m.Module
}
