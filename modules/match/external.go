package match

import (
	"context"
	"fmt"
	"gameserver/common/base/actor"
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
	Module  *internal.Module
	ChanRPC *chanrpc.Server
}

var External = &MatchExternal{}

func (m *MatchExternal) InitExternal(system *actor.ActorSystem) error {
	if system == nil {
		return fmt.Errorf("match: ActorSystem is nil")
	}
	scope, err := system.NewScope("match")
	if err != nil {
		return err
	}
	m.ChanRPC = internal.ChanRPC
	matchManager, err := managers.NewMatchManager(
		context.Background(),
		scope,
		game.NewMatchPlayerReader(),
		room.External,
		func() string { return strconv.FormatInt(utils.FlakeId(), 10) },
		time.Now,
	)
	if err != nil {
		return err
	}
	m.Module = internal.NewModule(scope, schedule.NewScheduler(), matchManager, game.External.TeamManager)
	return nil
}

func (m *MatchExternal) GetModule() module.Module {
	return m.Module
}
