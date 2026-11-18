package internal

import (
	"context"
	"gameserver/common"
	"gameserver/common/base/actor"
	"gameserver/common/schedule"
	"gameserver/core/log"
	"gameserver/core/module"
	"gameserver/modules/match/internal/handlers"
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
	manager   matchManager
	teams     handlers.TeamMessenger
	job       schedule.Job
}

type matchManager interface {
	handlers.MatchManager
	OnTimer()
}

func NewModule(scope *actor.Scope, scheduler schedule.Scheduler, manager matchManager, teams handlers.TeamMessenger) *Module {
	return &Module{scope: scope, scheduler: scheduler, manager: manager, teams: teams}
}

func (m *Module) OnInit() {
	m.Skeleton = skeleton
	InitHandler(m.manager, m.teams)
	m.job = m.scheduler.Every(10*time.Second, m.manager.OnTimer)
}

func (m *Module) OnDestroy() {
	stopContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.job.Stop(stopContext); err != nil {
		log.Error("停止 match 定时任务失败: %v", err)
	}
	if err := m.scope.Stop(stopContext); err != nil {
		log.Error("停止 match Actor Scope 失败: %v", err)
	}
}
