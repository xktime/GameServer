package player

import (
	"gameserver/core/gate"
)

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
