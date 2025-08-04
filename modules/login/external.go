package login

import (
	"gameserver/core/chanrpc"
	"gameserver/core/module"
	"gameserver/modules/login/internal"
	"gameserver/modules/login/internal/managers"
)

type LoginExternal struct {
	Module  *internal.Module
	ChanRPC *chanrpc.Server
	LoginManager *managers.LoginManager
}

var External = &LoginExternal{}

func (m *LoginExternal) InitExternal() {
	m.Module = new(internal.Module)
	m.ChanRPC = internal.ChanRPC
	m.LoginManager = managers.GetLoginManager()
}

func (m *LoginExternal) GetModule() module.Module {
	return m.Module
}
