package player

import (
	"gameserver/common/models"
	actor_manager "gameserver/core/actor"
	"gameserver/core/gate"
)

func ActorRegister[T any](playerId int64, initFunc ...func(*T)) (*actor_manager.ActorMeta[T], error) {
	return actor_manager.Register[T](playerId, actor_manager.Player, initFunc...)
}

// todo 需要初始化所有的玩家模块
func Login(agent gate.Agent, isNew bool) {
	initModules(agent, isNew)
}

func initModules(agent gate.Agent, isNew bool) {
	PlayerInit(agent, isNew)
	Print(agent.UserData().(models.User).PlayerId)
}
