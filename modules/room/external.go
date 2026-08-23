package room

import (
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

func (m *RoomExternal) InitExternal() {
	if game.External.TeamManager == nil {
		panic("room: RoomExternal.InitExternal called before GameExternal.InitExternal")
	}
	m.ChanRPC = internal.ChanRPC
	m.RoomManager = managers.RegisterRoomManager(
		game.NewRoomTeamProjection(game.External.TeamManager),
		game.NewRoomPlayerMessenger(),
		time.Now,
		uuid.NewString,
	)
	m.Module = internal.NewModule(schedule.NewScheduler(), m.RoomManager)
	event_dispatcher.RegisterDispatcher(m.ChanRPC)
}

func (m *RoomExternal) GetModule() module.Module {
	return m.Module
}

func (m *RoomExternal) AcceptMatch(admission matchentry.Admission) matchentry.Acceptance {
	return m.RoomManager.AcceptMatch(admission)
}
