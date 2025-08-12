package login

import (
	"gameserver/common/event_dispatcher"
	"gameserver/core/chanrpc"
	"gameserver/core/module"
	"gameserver/modules/login/internal"
	"gameserver/modules/login/internal/managers"
)

type LoginExternal struct {
	Module       *internal.Module
	ChanRPC      *chanrpc.Server
	LoginManager *managers.LoginManagerActorProxy
}

var External = &LoginExternal{}

func (m *LoginExternal) InitExternal() {
	m.Module = new(internal.Module)
	m.ChanRPC = internal.ChanRPC
	m.LoginManager = managers.GetLoginManager()
	event_dispatcher.RegisterDispatcher(m.ChanRPC)
}

func (m *LoginExternal) GetModule() module.Module {
	return m.Module
}
