package match

import (
	"gameserver/common/event_dispatcher"
	"gameserver/core/chanrpc"
	"gameserver/core/module"
	"gameserver/modules/match/internal"
	"gameserver/modules/match/internal/managers"
)

type MatchExternal struct {
	Module       *internal.Module
	ChanRPC      *chanrpc.Server
	MatchManager *managers.MatchManagerActorProxy
}

var External = &MatchExternal{}

func (m *MatchExternal) InitExternal() {
	m.Module = new(internal.Module)
	m.ChanRPC = internal.ChanRPC
	m.MatchManager = managers.GetMatchManager()
	event_dispatcher.RegisterDispatcher(m.ChanRPC)
}

func (m *MatchExternal) GetModule() module.Module {
	return m.Module
}
