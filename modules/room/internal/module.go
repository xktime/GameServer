package internal

import (
	"gameserver/common"
	"gameserver/core/module"
	"gameserver/modules/room/internal/managers"
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
	managers.GetRoomManager().Stop()
}
