package login

import (
	"gameserver/core/chanrpc"
	"gameserver/core/module"
	"gameserver/modules/login/internal"
)

type LoginExternal struct {
	Module  *internal.Module
	ChanRPC *chanrpc.Server
}

var External = &LoginExternal{}

func (m *LoginExternal) InitExternal() {
	m.Module = new(internal.Module)
	m.ChanRPC = internal.ChanRPC
}

func (m *LoginExternal) GetModule() module.Module {
	return m.Module
}
