package player

import (
	"gameserver/common/models"
	actor_manager "gameserver/core/actor"
)

func ActorRegister[T any](playerId int64, initFunc ...func(*T)) (*actor_manager.ActorMeta[T], error)  {
	return actor_manager.Register[T](playerId, actor_manager.Player, initFunc...)
}

// todo 需要初始化所有的玩家模块
func Login(user models.User, isNew bool) {
	initModules(user, isNew)
}

func initModules(user models.User, isNew bool) {
	PlayerInit(user, isNew)

//	actor_manager.GetActor[Player](user.PlayerId).PrintByActor()
	actor_manager.Send[Player](user.PlayerId, "Print", nil)
}