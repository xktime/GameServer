package internal

import (
	"context"
	"gameserver/common"
	"gameserver/common/base/actor"
	"gameserver/common/schedule"
	"gameserver/core/log"
	"gameserver/core/module"
	"gameserver/modules/login/internal/handlers"
	"time"
)

var (
	skeleton = common.NewSkeleton()
	ChanRPC  = skeleton.ChanRPCServer
)

type Module struct {
	*module.Skeleton
	scope       *actor.Scope
	scheduler   schedule.Scheduler
	login       handlers.LoginManager
	connections connectionManager
	job         schedule.Job
}

type connectionManager interface {
	handlers.ConnectManager
	OnTimer()
}

func NewModule(scope *actor.Scope, scheduler schedule.Scheduler, login handlers.LoginManager, connections connectionManager) *Module {
	return &Module{scope: scope, scheduler: scheduler, login: login, connections: connections}
}

func (m *Module) OnInit() {
	m.Skeleton = skeleton
	InitHandler(m.login, m.connections)
	m.job = m.scheduler.Every(10*time.Second, m.connections.OnTimer)
}

func (m *Module) OnDestroy() {
	m.job.Stop()
	if err := m.scope.Stop(context.Background()); err != nil {
		log.Error("停止 login Actor Scope 失败: %v", err)
	}
}
