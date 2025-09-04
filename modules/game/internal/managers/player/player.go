package player

import (
	"fmt"
	"gameserver/common/db/mongodb"
	"gameserver/common/models"
	"gameserver/common/msg/message"
	actor_manager "gameserver/core/actor"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game/internal/managers/team"
	"gameserver/modules/game/internal/models/player"

	"google.golang.org/protobuf/proto"
)

type Player struct {
	actor_manager.ActorMessageHandler `bson:"-"`
	PlayerId                          int64              `bson:"_id"`
	PlayerInfo                        *player.PlayerInfo `bson:"player_info"`
	TeamId                            int64              `bson:"team_id"`
	agent                             gate.Agent         `bson:"-"`
}

func (p Player) GetPersistId() interface{} {
	return p.PlayerId
}

// 玩家模块
func InitPlayer(agent gate.Agent, isNew bool) *Player {
	user := agent.UserData().(models.User)
	playerId := user.PlayerId

	// 检查是否已存在Actor
	if existingMeta := actor_manager.GetMeta[Player](playerId); existingMeta != nil {
		log.Error("玩家Actor已存在，可能是离线未正常清理: %v", playerId)
		// 可以选择清理旧的Actor或直接返回
		actor_manager.StopGroup(actor_manager.Player, playerId)
	}

	// 初始化玩家数据
	p, err := initPlayerData(playerId, user, isNew)
	if err != nil {
		log.Error("初始化玩家数据失败: %v", err)
		return nil
	}

	// 注册Actor
	meta, err := PlayerActorRegister(playerId, func(a *Player) {
		a.PlayerId = playerId
		a.PlayerInfo = p.PlayerInfo
		a.TeamId = p.TeamId
		a.agent = agent
	})
	if err != nil {
		log.Error("注册玩家Actor失败: %v", err)
		return nil
	}

	return meta.Actor
}

// initPlayerData 初始化玩家数据
func initPlayerData(playerId int64, user models.User, isNew bool) (*Player, error) {
	if isNew {
		// 新玩家：创建初始数据
		playerInfo := &player.PlayerInfo{
			ServerId: user.ServerId,
		}

		// 保存新玩家数据
		player := &Player{
			PlayerId:   playerId,
			PlayerInfo: playerInfo,
		}
		if _, err := mongodb.Save(player); err != nil {
			return nil, err
		}

		return player, nil
	} else {
		// 老玩家：从数据库加载数据
		existingPlayer, err := mongodb.FindOneById[Player](playerId)
		if err != nil {
			return nil, err
		}
		if existingPlayer == nil {
			return nil, fmt.Errorf("老玩家数据不存在: %v", playerId)
		}

		return existingPlayer, nil
	}
}

func (p *Player) ModifyName(name string) message.Result {
	if len(name) < 2 || len(name) > 20 {
		return message.Result_Illegal
	}
	p.PlayerInfo.PlayerName = name
	return message.Result_Success
}

func (p *Player) InitTeam() {
	if p.TeamId != 0 {
		teamActor := actor_manager.Get[team.Team](p.TeamId)
		if teamActor != nil {
			// todo 重连房间
			if teamActor.RoomId > 0 {
				return
			}
		}
	}
	teamInfo := team.InitTeam(p.agent)
	p.TeamId = teamInfo.TeamId
	team.JoinTeam(p.TeamId, p.PlayerId)

}

func (p *Player) SendToClient(message proto.Message) {
	p.agent.WriteMsg(message)
}

func (p *Player) CloseAgent() {
	p.agent.Close()
}
