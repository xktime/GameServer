package managers

import (
	actor_manager "gameserver/core/actor"
	"sync"
)

type LoginManager struct {
	actor_manager.ActorMessageHandler
}

var (
	loginManager *LoginManager
	loginOnce    sync.Once
)

func GetLoginManager() *LoginManager {
	loginOnce.Do(func() {
		meta, _ := actor_manager.Register[LoginManager]("1", actor_manager.Login)
		loginManager = &LoginManager{
			ActorMessageHandler: *actor_manager.NewActorMessageHandler(meta),
		}
	})
	return loginManager
}
