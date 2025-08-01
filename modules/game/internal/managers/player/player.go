package player

import (
	"encoding/json"
	"gameserver/common/db/mongodb"
	"gameserver/common/models"
	actor_manager "gameserver/core/actor"
	"gameserver/core/log"
	"gameserver/modules/game/internal/models/player"
)

type Player struct {
	actor_manager.ActorMessageHandler
	PlayerInfo *player.PlayerInfo
}

// 玩家模块
func PlayerInit(user models.User, isNew bool) *Player {
	var p *player.PlayerInfo
	playerId := user.PlayerId
	if isNew {
		p = &player.PlayerInfo{
			PlayerId: playerId,
			ServerId: user.ServerId,
		}
		mongodb.Save(p)
	} else {
		var err error
		p, err = mongodb.FindOneById[player.PlayerInfo](playerId)
		if err != nil {
			log.Error("PlayerInit find user failed: %v", err)
			return nil
		}
		if p == nil {
			log.Error("老玩家登录，player为空: %v", playerId)
			return nil
		}
	}

	meta, _ := ActorRegister[Player](playerId, func(a *Player) {
		a.PlayerInfo = p
	})
	return meta.Actor
}

func (p *Player) Print() {
	data, err := json.Marshal(p.PlayerInfo)
	if err != nil {
		log.Error("JSON 序列化错误: %v\n", err)
		return
	}
	log.Release("Print Player: %s", string(data))
}

func (p *Player) PrintByActor() {
	//	p.AddToActor("PrintByActor", nil)

}
