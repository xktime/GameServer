package game

import (
	"gameserver/core/chanrpc"
	"gameserver/core/module"
	"gameserver/modules/game/internal"
	"gameserver/modules/game/internal/managers"
)

type GameExternal struct {
	UserManager *managers.UserManager
	Module      *internal.Module
	ChanRPC     *chanrpc.Server
}

var External = &GameExternal{}

func (m *GameExternal) InitExternal() {
	m.UserManager = managers.GetUserManager()
	m.Module = new(internal.Module)
	m.ChanRPC = internal.ChanRPC
}

func (m *GameExternal) GetModule() module.Module {
	return m.Module
}
