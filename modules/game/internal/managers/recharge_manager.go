package managers

import (
	actor_manager "gameserver/core/actor"
	"sync"
)

type RechargeManager struct {
	actor_manager.ActorMessageHandler `bson:"-"`
}

var (
	rechargeMeta *actor_manager.ActorMeta[RechargeManager]
	rechargeOnce sync.Once
)

func GetRechargeManager() *RechargeManager {
	userOnce.Do(func() {
		rechargeMeta, _ = actor_manager.Register[RechargeManager]("1", actor_manager.Recharge)
	})
	return rechargeMeta.Actor
}
