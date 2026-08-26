package module

import "gameserver/common/base/actor"

type External interface {
	InitExternal(*actor.ActorSystem) error
	GetModule() Module
}
