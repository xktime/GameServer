package internal

import (
	"gameserver/common"
	"gameserver/common/base/actor"
	"gameserver/core/module"
)

var (
	skeleton = common.NewSkeleton()
	ChanRPC  = skeleton.ChanRPCServer
)

type Module struct {
	*module.Skeleton
}

func (m *Module) OnInit() {
	m.Skeleton = skeleton
	InitHandler()
}

func (m *Module) OnDestroy() {
	actor.StopAll()
}
