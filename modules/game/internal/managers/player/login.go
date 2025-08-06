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
// todo Actor直接存actor还是每个结构自己另存？直接存可能会导致读写性能问题；
// 另存会很麻烦，actor save的时候需要操作多次数据库
func Login(agent gate.Agent, isNew bool) {
	initModules(agent, isNew)
}

func initModules(agent gate.Agent, isNew bool) {
	PlayerInit(agent, isNew)
	Print(agent.UserData().(models.User).PlayerId)
}
