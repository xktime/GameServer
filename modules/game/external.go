package game

import (
	"context"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/event_dispatcher"
	"gameserver/core/chanrpc"
	"gameserver/core/module"
	"gameserver/modules/game/internal"
	"gameserver/modules/game/internal/managers"
	"time"
)

type GameExternal struct {
	UserManager *managers.UserManager
	TeamManager *managers.TeamManager
	Module      *internal.Module
	ChanRPC     *chanrpc.Server
}

var External = &GameExternal{}

func (m *GameExternal) InitExternal(system *actor.ActorSystem) error {
	if system == nil {
		return fmt.Errorf("game: ActorSystem is nil")
	}
	scope, err := system.NewScope("game")
	if err != nil {
		return err
	}
	initializationContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	m.UserManager, m.TeamManager, err = managers.NewManagers(initializationContext, scope)
	if err != nil {
		return err
	}
	m.Module = internal.NewModule(scope, m.UserManager)
	m.ChanRPC = internal.ChanRPC
	event_dispatcher.RegisterDispatcher(m.ChanRPC)
	return nil
}

func (m *GameExternal) GetModule() module.Module {
	return m.Module
}
