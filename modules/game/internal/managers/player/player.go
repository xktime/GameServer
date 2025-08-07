package player

import (
	"encoding/json"
	"gameserver/common/db/mongodb"
	"gameserver/common/models"
	"gameserver/common/msg/message"
	actor_manager "gameserver/core/actor"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game/internal/models/player"

	"google.golang.org/protobuf/proto"
)

type Player struct {
	actor_manager.ActorMessageHandler `bson:"-"`
	PlayerId                          int64              `bson:"_id"`
	PlayerInfo                        *player.PlayerInfo `bson:"player_info"`
	agent                             gate.Agent         `bson:"-"`
}

func (p Player) GetPersistId() interface{} {
	return p.PlayerId
}

// 玩家模块
func PlayerInit(agent gate.Agent, isNew bool) *Player {
	var p *player.PlayerInfo
	user := agent.UserData().(models.User)
	playerId := user.PlayerId
	if isNew {
		p = &player.PlayerInfo{
			ServerId: user.ServerId,
		}
	} else {
		// var err error
		mongodb.FindOneById[Player](playerId)
		// if err != nil {
		// 	log.Error("PlayerInit find user failed: %v", err)
		// 	return nil
		// }
		// if p == nil {
		// 	log.Error("老玩家登录，player为空: %v", playerId)
		// 	return nil
		// }
	}

	meta, _ := ActorRegister[Player](playerId, func(a *Player) {
		a.PlayerId = playerId
		a.PlayerInfo = p
		a.agent = agent
	})
	if isNew {
		mongodb.Save(meta.Actor)
	}
	// meta离线没有正常删除，还存在
	if meta == nil {
		meta = actor_manager.GetMeta[Player](playerId)
		log.Error("玩家离线没有正常移除Actor缓存:%v", playerId)
	}
	return meta.Actor
}

func (p *Player) Print() {
	data, err := json.Marshal(p.PlayerInfo)
	if err != nil {
		log.Error("JSON 序列化错误: %v\n", err)
		return
	}
	log.Release("Print Player: %s", string(data))
	p.SendToClient(&message.S2C_Login{LoginResult: -1})
}

func (p *Player) SendToClient(message proto.Message) {
	p.agent.WriteMsg(message)
}

func (p *Player) PrintJson(json string) {
	log.Release("Print Player: %s", json)
}
