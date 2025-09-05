package game

import (
	"gameserver/common/event_dispatcher"
	"gameserver/core/chanrpc"
	"gameserver/core/module"
	"gameserver/modules/game/internal"
	"gameserver/modules/game/internal/managers"
)

type GameExternal struct {
	UserManager *managers.UserManager
	TeamManager *managers.TeamManager
	Module      *internal.Module
	ChanRPC     *chanrpc.Server
}

var External = &GameExternal{}

func (m *GameExternal) InitExternal() {
	m.UserManager = managers.GetUserManager()
	m.TeamManager = managers.GetTeamManager()
	m.Module = new(internal.Module)
	m.ChanRPC = internal.ChanRPC
	event_dispatcher.RegisterDispatcher(m.ChanRPC)
}

func (m *GameExternal) GetModule() module.Module {
	return m.Module
}
