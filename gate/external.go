package gate

import (
	"gameserver/common/base/actor"
	"gameserver/core/module"
	"gameserver/gate/internal"
)

type GateExternal struct {
	Module *internal.Module
}

var External = &GateExternal{}

func (m *GateExternal) InitExternal(*actor.ActorSystem) error {
	m.Module = new(internal.Module)
	InitRouter()
	return nil
}

func (m *GateExternal) GetModule() module.Module {
	return m.Module
}
