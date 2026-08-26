package internal

import (
	"context"
	"gameserver/common"
	"gameserver/common/base/actor"
	"gameserver/common/schedule"
	"gameserver/core/log"
	"gameserver/core/module"
	"gameserver/modules/rank/internal/handlers"
	"time"
)

var (
	skeleton = common.NewSkeleton()
	ChanRPC  = skeleton.ChanRPCServer
)

type Module struct {
	*module.Skeleton
	scope     *actor.Scope
	scheduler schedule.Scheduler
	manager   rankManager
	job       schedule.Job
}

type rankManager interface {
	handlers.RankManager
	CheckCrossDay()
}

func NewModule(scope *actor.Scope, scheduler schedule.Scheduler, manager rankManager) *Module {
	return &Module{scope: scope, scheduler: scheduler, manager: manager}
}

func (m *Module) OnInit() {
	m.Skeleton = skeleton
	InitHandler(m.manager)
	m.job = m.scheduler.Every(time.Second, m.manager.CheckCrossDay)
}

func (m *Module) OnDestroy() {
	m.job.Stop()
	if err := m.scope.Stop(context.Background()); err != nil {
		log.Error("停止 rank Actor Scope 失败: %v", err)
	}
}
