package login

import (
	"context"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/event_dispatcher"
	"gameserver/common/schedule"
	"gameserver/core/chanrpc"
	"gameserver/core/module"
	"gameserver/modules/game"
	"gameserver/modules/login/internal"
	"gameserver/modules/login/internal/managers"
)

type LoginExternal struct {
	Module  *internal.Module
	ChanRPC *chanrpc.Server
}

var External = &LoginExternal{}

func (m *LoginExternal) InitExternal(system *actor.ActorSystem) error {
	if system == nil {
		return fmt.Errorf("login: ActorSystem is nil")
	}
	if game.External.UserManager == nil {
		return fmt.Errorf("login: GameExternal is not initialized")
	}
	scope, err := system.NewScope("login")
	if err != nil {
		return err
	}
	m.ChanRPC = internal.ChanRPC
	connectManager, err := managers.NewConnectManager(context.Background(), scope)
	if err != nil {
		return err
	}
	loginManager, err := managers.NewLoginManager(context.Background(), scope, game.External.UserManager)
	if err != nil {
		return err
	}
	m.Module = internal.NewModule(scope, schedule.NewScheduler(), loginManager, connectManager)
	event_dispatcher.RegisterDispatcher(m.ChanRPC)
	return nil
}

func (m *LoginExternal) GetModule() module.Module {
	return m.Module
}
