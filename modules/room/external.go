package room

import (
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/event_dispatcher"
	"gameserver/common/schedule"
	"gameserver/core/chanrpc"
	"gameserver/core/module"
	"gameserver/modules/game"
	"gameserver/modules/room/internal"
	"gameserver/modules/room/internal/managers"
	"gameserver/modules/room/matchentry"
	"time"

	"github.com/google/uuid"
)

type RoomExternal struct {
	Module      *internal.Module
	ChanRPC     *chanrpc.Server
	RoomManager *managers.RoomManager
}

var External = &RoomExternal{}

func (m *RoomExternal) InitExternal(system *actor.ActorSystem) error {
	if system == nil {
		return fmt.Errorf("room: ActorSystem is nil")
	}
	if game.External.TeamManager == nil {
		return fmt.Errorf("room: GameExternal is not initialized")
	}
	scope, err := system.NewScope("room")
	if err != nil {
		return err
	}
	m.ChanRPC = internal.ChanRPC
	m.RoomManager, err = managers.NewRoomManager(
		scope,
		game.NewRoomTeamProjection(game.External.TeamManager),
		game.NewRoomPlayerMessenger(game.External.UserManager),
		time.Now,
		uuid.NewString,
	)
	if err != nil {
		return err
	}
	m.Module = internal.NewModule(scope, schedule.NewScheduler(), m.RoomManager)
	event_dispatcher.RegisterDispatcher(m.ChanRPC)
	return nil
}

func (m *RoomExternal) GetModule() module.Module {
	return m.Module
}

func (m *RoomExternal) AcceptMatch(admission matchentry.Admission) matchentry.Acceptance {
	return m.RoomManager.AcceptMatch(admission)
}
