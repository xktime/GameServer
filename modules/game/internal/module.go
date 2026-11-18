package internal

import (
	"context"
	"errors"
	"fmt"
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
	broadcasts      broadcastStopper
}

type provisionWorker interface {
	Start()
	Stop(context.Context) error
}

type broadcastStopper interface {
	StopBroadcasts(context.Context) error
}

func NewModule(scope *actor.Scope, users handlers.UserManager, worker provisionWorker, broadcasts broadcastStopper) *Module {
	return &Module{scope: scope, users: users, provisionWorker: worker, broadcasts: broadcasts}
}

func (m *Module) OnInit() {
	m.Skeleton = skeleton
	InitHandler(m.users)
	m.provisionWorker.Start()
}

func (m *Module) OnDestroy() {
	if err := m.Shutdown(); err != nil {
		log.Error("停止 game 模块失败: %v", err)
	}
}

func (m *Module) Shutdown() error {
	var failures []error
	stopContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.provisionWorker.Stop(stopContext); err != nil {
		log.Error("停止 game Player Provision Worker 失败: %v", err)
		finalContext, cancelFinal := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelFinal()
		if err := m.provisionWorker.Stop(finalContext); err != nil {
			failures = append(failures, fmt.Errorf("stop game player provision worker: %w", err))
		}
	}
	broadcastContext, cancelBroadcasts := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelBroadcasts()
	if err := m.broadcasts.StopBroadcasts(broadcastContext); err != nil {
		failures = append(failures, fmt.Errorf("stop game broadcasts: %w", err))
	}
	scopeContext, cancelScope := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelScope()
	if err := m.scope.Stop(scopeContext); err != nil {
		failures = append(failures, fmt.Errorf("stop game actor scope: %w", err))
	}
	return errors.Join(failures...)
}
