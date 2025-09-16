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
	actor.BaseActor `bson:"-"`
	PlayerId        int64              `bson:"_id"`
	PlayerInfo      *player.PlayerInfo `bson:"player_info"`
	TeamId          int64              `bson:"team_id"`
	TowerLevel      int32              `bson:"tower_level"`
	agent           gate.Agent         `bson:"-"`
}

func (p Player) GetPersistId() interface{} {
	return p.PlayerId
}

func DoPlayerLogin(agent gate.Agent, isNew bool) *Player {
	p := initPlayer(agent, isNew)
	if p == nil {
		agent.WriteMsg(&message.S2C_Login{
			LoginResult: -1,
		})
		agent.Close()
		log.Debug("UserLogin failed: %v", p)
		return nil
	}
	p.InitModules()
	return p
}

// 玩家模块
func initPlayer(agent gate.Agent, isNew bool) *Player {
	user := agent.UserData().(models.User)
	playerId := user.PlayerId

	// 检查是否已存在Actor
	// todo 登录频繁可能会导致登录不上
	if existingPlayer := GetPlayerActor(playerId); existingPlayer != nil {
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

	return actor.RegisterActor[*Player](actor.Player, playerId, agent, p)
}

func (p *Player) Init(args ...any) {
	if agent, ok := args[0].(gate.Agent); ok {
		p.agent = agent
	} else {
		log.Error("初始化玩家数据失败: %v", args[0])
	}
	if p1, ok := args[1].(*Player); ok {
		p.PlayerInfo = p1.PlayerInfo
		p.TeamId = p1.TeamId
		p.TowerLevel = p1.TowerLevel
		p.PlayerId = p1.PlayerId
	} else {
		log.Error("初始化玩家数据失败: %v", args[1])
	}
}

// InitModules 初始化玩家模块（装备、背包等）
func (p *Player) InitModules() {
	// 自己的actor里面可以放队列里初始化
	// 其他actor需要同步初始化，避免快速请求还未加载完成
	p.SendTaskAsync(func() *actor.Response {

		return nil
	})
	p.InitTeam()
}

// initPlayerData 初始化玩家数据
func initPlayerData(playerId int64, user models.User, isNew bool) (*Player, error) {
	if isNew {
		// 新玩家：创建初始数据
		playerInfo := &player.PlayerInfo{
			PlayerId:      playerId,
			ServerId:      user.ServerId,
			PlayerName:    user.OpenId,
			Level:         1,
			Balance:       0,
			TotalRecharge: 0,
			VipLevel:      0,
		}

		// 创建新玩家数据
		newPlayer := &Player{
			PlayerId:   playerId,
			PlayerInfo: playerInfo,
		}

		// 保存新玩家数据
		if _, err := mongodb.Save(newPlayer); err != nil {
			return nil, err
		}

		return newPlayer, nil
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
	p.PlayerInfo.PlayerName = name
	return message.Result_Success
}

func (p *Player) InitTeam() {
	if p.TeamId != 0 {
		teamActor, ok := actor.GetActor[team.Team](actor.Team, p.TeamId)
		// team还存在
		if ok {
			if teamActor.RoomId > 0 {
				// todo 重连逻辑
				return
			}
			return
		}
	}
	teamInfo := team.InitTeam(p.agent)
	p.TeamId = teamInfo.TeamId
	teamInfo.JoinTeam(p.PlayerId)
}

func SendToClient(playerId int64, message proto.Message) {
	player := GetPlayerActor(playerId)
	if player == nil {
		return
	}
	player.SendToClient(message)
}

func (p *Player) SendToClient(message proto.Message) {
	p.SendTask(func() *actor.Response {
		p.DoSendToClient(message)
		return nil
	})
}

func (p *Player) SendToClientSeq(message proto.Message, seq uint32) {
	p.SendTaskAsync(func() *actor.Response {
		p.agent.WriteMsgWithSeq(message, seq)
		return nil
	})
}

func (p *Player) DoSendToClient(message proto.Message) {
	p.agent.WriteMsg(message)
}

func (p *Player) CloseAgent() {
	p.agent.Close()
}
