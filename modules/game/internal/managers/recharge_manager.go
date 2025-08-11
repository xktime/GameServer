package managers

import (
	actor_manager "gameserver/core/actor"
)

type RechargeManager struct {
	actor_manager.ActorMessageHandler `bson:"-"`
}
