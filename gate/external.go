package gate

import (
	"gameserver/core/module"
	"gameserver/gate/internal"
)

type GateExternal struct {
	Module *internal.Module
}

var External = &GateExternal{}

func (m *GateExternal) InitExternal() {
	m.Module = new(internal.Module)
	InitRouter()
}

func (m *GateExternal) GetModule() module.Module {
	return m.Module
}
