package player

import (
	actor_manager "gameserver/core/actor"
	"gameserver/core/gate"
)

func PlayerActorRegister[T any](playerId int64, initFunc ...func(*T)) (*actor_manager.ActorMeta[T], error) {
	return actor_manager.Register(playerId, actor_manager.Player, initFunc...)
}

func Login(agent gate.Agent, isNew bool) *Player {
	p := initModules(agent, isNew)
	return p
}

// todo 需要初始化所有的玩家模块
func initModules(agent gate.Agent, isNew bool) *Player {
	player := InitPlayer(agent, isNew)
	player.InitTeam()
	return player
}
