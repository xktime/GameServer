package internal

import (
	"gameserver/common/models"
	"gameserver/core/gate"
	"gameserver/modules/game"
	"gameserver/modules/match/internal/managers/room"
)

func init() {
	skeleton.RegisterChanRPC("CloseAgent", rpcCloseAgent)
}

func rpcCloseAgent(args []interface{}) {
	a := args[0].(gate.Agent)
	user := a.UserData()
	if user != nil {
		playerId := user.(models.User).PlayerId
		team := game.External.TeamManager.GetTeamByPlayerId(playerId)
		if team == nil {
			return
		}
		room.PlayerOffline(team.RoomId, playerId)
	}
	_ = a
}
