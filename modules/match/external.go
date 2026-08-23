package match

import (
	"gameserver/common/utils"
	"gameserver/core/chanrpc"
	"gameserver/core/module"
	"gameserver/modules/game"
	"gameserver/modules/match/internal"
	"gameserver/modules/match/internal/managers"
	"gameserver/modules/room"
	"strconv"
)

type MatchExternal struct {
	Module       *internal.Module
	ChanRPC      *chanrpc.Server
	MatchManager *managers.MatchManager
}

var External = &MatchExternal{}

func (m *MatchExternal) InitExternal() {
	m.Module = new(internal.Module)
	m.ChanRPC = internal.ChanRPC
	m.MatchManager = managers.RegisterMatchManager(
		game.NewMatchPlayerReader(),
		room.External,
		func() string { return strconv.FormatInt(utils.FlakeId(), 10) },
	)
}

func (m *MatchExternal) GetModule() module.Module {
	return m.Module
}
