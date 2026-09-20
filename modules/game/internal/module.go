package internal

import (
	"context"
	"gameserver/common"
	"gameserver/common/base/actor"
	"gameserver/core/log"
	"gameserver/core/module"
	"gameserver/modules/game/internal/handlers"
	"time"
)

var (
	skeleton = common.NewSkeleton()
	ChanRPC  = skeleton.ChanRPCServer
)

type Module struct {
	*module.Skeleton
	scope           *actor.Scope
	users           handlers.UserManager
	provisionWorker provisionWorker
}

type provisionWorker interface {
	Start()
	Stop(context.Context) error
}

func NewModule(scope *actor.Scope, users handlers.UserManager, worker provisionWorker) *Module {
	return &Module{scope: scope, users: users, provisionWorker: worker}
}

func (m *Module) OnInit() {
	m.Skeleton = skeleton
	InitHandler(m.users)
	m.provisionWorker.Start()
}

func (m *Module) OnDestroy() {
	stopContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := m.provisionWorker.Stop(stopContext); err != nil {
		log.Error("停止 game Player Provision Worker 失败: %v", err)
	}
	cancel()
	if err := m.scope.Stop(context.Background()); err != nil {
		log.Error("停止 game Actor Scope 失败: %v", err)
	}
}
