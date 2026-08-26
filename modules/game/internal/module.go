package internal

import (
	"context"
	"gameserver/common"
	"gameserver/common/base/actor"
	"gameserver/core/log"
	"gameserver/core/module"
	"gameserver/modules/game/internal/handlers"
)

var (
	skeleton = common.NewSkeleton()
	ChanRPC  = skeleton.ChanRPCServer
)

type Module struct {
	*module.Skeleton
	scope *actor.Scope
	users handlers.UserManager
}

func NewModule(scope *actor.Scope, users handlers.UserManager) *Module {
	return &Module{scope: scope, users: users}
}

func (m *Module) OnInit() {
	m.Skeleton = skeleton
	InitHandler(m.users)
}

func (m *Module) OnDestroy() {
	if err := m.scope.Stop(context.Background()); err != nil {
		log.Error("停止 game Actor Scope 失败: %v", err)
	}
}
