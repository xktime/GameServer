package player

import (
	"gameserver/common/base/actor"
)

func GetPlayerActor(playerId int64) *Player {
	if bagActor, ok := actor.GetActor[Player](actor.Player, playerId); ok {
		return bagActor
	}
	return nil
}
