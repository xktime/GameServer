package player

import (
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/db/mongodb"
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game/internal/managers/team"
	"gameserver/modules/game/internal/models/player"

	"google.golang.org/protobuf/proto"
)

type Player struct {
	*actor.TaskHandler `bson:"-"`
	PlayerId           int64              `bson:"_id"`
	PlayerInfo         *player.PlayerInfo `bson:"player_info"`
	TeamId             int64              `bson:"team_id"`
	agent              gate.Agent         `bson:"-"`
}

func (p Player) GetPersistId() interface{} {
	return p.PlayerId
}

// 玩家模块
func InitPlayer(agent gate.Agent, isNew bool) *Player {
	user := agent.UserData().(models.User)
	playerId := user.PlayerId

	// 检查是否已存在Actor
	if existingPlayer, ok := actor.GetActor[Player](actor.Player, playerId); ok {
		log.Error("玩家Actor已存在，可能是离线未正常清理: %v", playerId)
		// 异步停止旧的Actor，避免在TaskHandler上下文中调用Stop造成死锁
		go func() {
			existingPlayer.Stop()
		}()
	}

	// 初始化玩家数据
	p, err := initPlayerData(playerId, user, isNew)
	if err != nil {
		log.Error("初始化玩家数据失败: %v", err)
		return nil
	}

	p.TaskHandler = actor.InitTaskHandler(actor.Player, playerId, p)
	p.agent = agent
	p.Init()
	return p
}

func (p *Player) Init() {
	p.TaskHandler.Start()
}

func (p *Player) Stop() {
	p.TaskHandler.Stop()
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
	response := p.SendTask(func() *actor.Response {
		result := p.doModifyName(name)
		return &actor.Response{
			Result: []interface{}{result},
		}
	})

	if response != nil && len(response.Result) > 0 {
		if result, ok := response.Result[0].(message.Result); ok {
			return result
		}
	}
	return message.Result_Fail
}

func (p *Player) doModifyName(name string) message.Result {
	if len(name) < 2 || len(name) > 20 {
		return message.Result_Illegal
	}
	if len(name) < 2 || len(name) > 20 {
		return message.Result_Illegal
	}
	p.PlayerInfo.PlayerName = name
	return message.Result_Success
}

func (p *Player) InitTeam() {
	// 直接调用，避免在TaskHandler上下文中再次调用SendTask造成死锁
	p.doInitTeam()
}

func (p *Player) doInitTeam() {
	if p.TeamId != 0 {
		teamActor, ok := actor.GetActor[team.Team](actor.Team, p.TeamId)
		if !ok {
			return
		}
		// todo 重连房间
		if teamActor.RoomId > 0 {
			return
		}
	}
	teamInfo := team.InitTeam(p.agent)
	p.TeamId = teamInfo.TeamId
	// 直接调用，避免在TaskHandler上下文中再次调用SendTask造成死锁
	// teamInfo.doJoinTeam(p.PlayerId)
}

func (p *Player) SendToClient(message proto.Message) {
	p.agent.WriteMsg(message)
}

func (p *Player) CloseAgent() {
	p.agent.Close()
}
