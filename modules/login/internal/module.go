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
	stopContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.job.Stop(stopContext); err != nil {
		log.Error("停止 login 定时任务失败: %v", err)
	}
	if err := m.login.Close(stopContext); err != nil {
		log.Error("停止 login 认证工作池失败: %v", err)
		// 认证池不属于 ActorSystem；在下游状态和存储关闭前继续等待退出。
		finalContext, cancelFinal := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelFinal()
		if err := m.login.Close(finalContext); err != nil {
			log.Error("等待 login 认证工作池退出失败: %v", err)
		}
	}
	if err := m.scope.Stop(stopContext); err != nil {
		log.Error("停止 login Actor Scope 失败: %v", err)
	}
}
