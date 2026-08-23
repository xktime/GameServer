package internal

import (
	"gameserver/common"
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
	scheduler   schedule.Scheduler
	maintenance roomMaintenance
	job         schedule.Job
}

type roomMaintenance interface {
	Maintain()
	Stop()
}

func NewModule(scheduler schedule.Scheduler, maintenance roomMaintenance) *Module {
	return &Module{scheduler: scheduler, maintenance: maintenance}
}

func (m *Module) OnInit() {
	m.Skeleton = skeleton
	InitHandler()
	m.job = m.scheduler.Every(10*time.Second, m.maintenance.Maintain)
}

func (m *Module) OnDestroy() {
	m.job.Stop()
	m.maintenance.Stop()
}
