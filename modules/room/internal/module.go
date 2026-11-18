package internal

import (
	"context"
	"errors"
	"fmt"
	"gameserver/common"
	"gameserver/common/base/actor"
	"gameserver/common/schedule"
	"gameserver/core/log"
	"gameserver/core/module"
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
	maintenance roomMaintenance
	job         schedule.Job
}

type roomMaintenance interface {
	roomManager
	Maintain()
	Stop(context.Context) error
}

func NewModule(scope *actor.Scope, scheduler schedule.Scheduler, maintenance roomMaintenance) *Module {
	return &Module{scope: scope, scheduler: scheduler, maintenance: maintenance}
}

func (m *Module) OnInit() {
	m.Skeleton = skeleton
	InitHandler(m.maintenance)
	m.job = m.scheduler.Every(10*time.Second, m.maintenance.Maintain)
}

func (m *Module) OnDestroy() {
	if err := m.Shutdown(); err != nil {
		log.Error("停止 room 模块失败: %v", err)
	}
}

func (m *Module) Shutdown() error {
	var failures []error
	stopContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.job.Stop(stopContext); err != nil {
		failures = append(failures, fmt.Errorf("stop room maintenance job: %w", err))
	}
	if err := m.maintenance.Stop(stopContext); err != nil {
		// 普通投影错误也可能恢复，必须在 game 保存 Team 前重试。
		retryContext, cancelRetry := context.WithTimeout(context.Background(), 5*time.Second)
		retryErr := m.maintenance.Stop(retryContext)
		cancelRetry()
		if retryErr != nil {
			failures = append(failures, fmt.Errorf("stop room manager: %w", errors.Join(err, retryErr)))
		}
	}
	scopeContext, cancelScope := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelScope()
	if err := m.scope.Stop(scopeContext); err != nil {
		failures = append(failures, fmt.Errorf("stop room actor scope: %w", err))
	}
	return errors.Join(failures...)
}
