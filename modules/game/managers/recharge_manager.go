package managers

import (
	actor_manager "gameserver/core/actor"
	"sync"
)

type RechargeManager struct {
	actor_manager.ActorMessageHandler
}

var (
	rechargeManager *RechargeManager
	rechargeOnce    sync.Once
)

func GetRechargeManager() *RechargeManager {
	rechargeOnce.Do(func() {
		meta, _ := actor_manager.Register[RechargeManager]("1", actor_manager.Recharge)
		rechargeManager = &RechargeManager{
			ActorMessageHandler: *actor_manager.NewActorMessageHandler(meta),
		}
	})
	return rechargeManager
}
