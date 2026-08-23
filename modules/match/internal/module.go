package internal

import (
	"gameserver/common"
	"gameserver/common/base/actor"
	"gameserver/common/schedule"
	"gameserver/core/module"
	"time"
)

var (
	skeleton = common.NewSkeleton()
	ChanRPC  = skeleton.ChanRPCServer
)

type Module struct {
	*module.Skeleton
	scheduler schedule.Scheduler
	cycle     matchCycle
	job       schedule.Job
}

type matchCycle interface {
	OnTimer()
}

func NewModule(scheduler schedule.Scheduler, cycle matchCycle) *Module {
	return &Module{scheduler: scheduler, cycle: cycle}
}

func (m *Module) OnInit() {
	m.Skeleton = skeleton
	InitHandler()
	m.job = m.scheduler.Every(10*time.Second, m.cycle.OnTimer)
}

func (m *Module) OnDestroy() {
	m.job.Stop()
	actor.StopAll()
}
