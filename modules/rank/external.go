package rank

import (
	"context"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/event_dispatcher"
	"gameserver/common/schedule"
	"gameserver/core/chanrpc"
	"gameserver/core/module"
	"gameserver/modules/game"
	"gameserver/modules/rank/internal"
	"gameserver/modules/rank/internal/managers"
	"time"
)

type RankExternal struct {
	Module  *internal.Module
	ChanRPC *chanrpc.Server
}

var External = &RankExternal{}

func (m *RankExternal) InitExternal(system *actor.ActorSystem) error {
	if system == nil {
		return fmt.Errorf("rank: ActorSystem is nil")
	}
	scope, err := system.NewScope("rank")
	if err != nil {
		return err
	}
	m.ChanRPC = internal.ChanRPC
	initializationContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rankManager, err := managers.NewRankManager(initializationContext, scope, game.NewRankPlayerReader())
	if err != nil {
		return err
	}
	m.Module = internal.NewModule(scope, schedule.NewScheduler(), rankManager)
	event_dispatcher.RegisterDispatcher(m.ChanRPC)
	return nil
}

func (m *RankExternal) GetModule() module.Module {
	return m.Module
}
