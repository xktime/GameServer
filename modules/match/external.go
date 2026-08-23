package match

import (
	"gameserver/common/schedule"
	"gameserver/common/utils"
	"gameserver/core/chanrpc"
	"gameserver/core/module"
	"gameserver/modules/game"
	"gameserver/modules/match/internal"
	"gameserver/modules/match/internal/managers"
	"gameserver/modules/room"
	"strconv"
	"time"
)

type MatchExternal struct {
	Module       *internal.Module
	ChanRPC      *chanrpc.Server
	MatchManager *managers.MatchManager
}

var External = &MatchExternal{}

func (m *MatchExternal) InitExternal() {
	m.ChanRPC = internal.ChanRPC
	m.MatchManager = managers.RegisterMatchManager(
		game.NewMatchPlayerReader(),
		room.External,
		func() string { return strconv.FormatInt(utils.FlakeId(), 10) },
		time.Now,
	)
	m.Module = internal.NewModule(schedule.NewScheduler(), m.MatchManager)
}

func (m *MatchExternal) GetModule() module.Module {
	return m.Module
}
