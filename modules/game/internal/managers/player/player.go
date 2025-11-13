package player

import (
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/bucket"
	"gameserver/common/db/mongodb"
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game/internal/managers/team"
	"gameserver/modules/game/internal/models/player"
	"path/filepath"
	"strconv"

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
	if existingPlayer := GetPlayerActor(playerId); existingPlayer != nil {
		log.Error("玩家Actor已存在，可能是离线未正常清理: %v", playerId)
		existingPlayer.Stop()
	}

	return actor.RegisterActor[*Player](actor.Player, playerId, agent, user, isNew)
}

func (p *Player) Init(args ...any) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("初始化玩家数据失败: %v", r)
			p.PlayerId = 0
		}
	}()
	agent := args[0].(gate.Agent)
	user := args[1].(models.User)
	isNew := args[2].(bool)
	playerId := user.PlayerId
	// 初始化玩家数据
	err := p.InitPlayerData(playerId, user, isNew)
	if err != nil {
		log.Error("初始化玩家数据失败: %v", err)
		return
	}
	p.agent = agent
}

// InitModules 初始化玩家模块
func (p *Player) InitModules() {
	// 自己的actor里面可以放队列里初始化
	// 其他actor需要同步初始化，避免快速请求还未加载完成
	p.SendTaskAsync(func() {
	})
	p.InitTeam()
}

// initPlayerData 初始化玩家数据
func (p *Player) InitPlayerData(playerId int64, user models.User, isNew bool) error {
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
		p.PlayerId = playerId
		p.PlayerInfo = playerInfo

		// newPlayer.initAvatar()
		// 保存新玩家数据
		if _, err := mongodb.Save(newPlayer); err != nil {
			return err
		}
	} else {
		// 老玩家：从数据库加载数据
		existingPlayer, err := mongodb.FindOneById[Player](playerId)
		if err != nil {
			return err
		}
		if existingPlayer == nil {
			return fmt.Errorf("老玩家数据不存在: %v", playerId)
		}

		p.PlayerInfo = existingPlayer.PlayerInfo
		p.TeamId = existingPlayer.TeamId
		p.TowerLevel = existingPlayer.TowerLevel
		p.PlayerId = existingPlayer.PlayerId
	}
	fmt.Println("InitPlayerData", p.PlayerInfo.GetAvatarURL())
	return nil
}

func (p *Player) ModifyName(name string) message.Result {
	result := p.SendTask(func() message.Result {
		return p.doModifyName(name)
	})

	if err, ok := result.(error); ok {
		log.Error("修改名字失败: %v", err)
		return message.Result_Fail
	}

	if typedResult, ok := result.(message.Result); ok {
		return typedResult
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
			if teamActor.RoomId != "" {
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
	p.SendTask(func() {
		p.DoSendToClient(message)
	})
}

func (p *Player) SendToClientSeq(message proto.Message, seq uint32) {
	p.SendTaskAsync(func() {
		p.agent.WriteMsgWithSeq(message, seq)
	})
}

func (p *Player) DoSendToClient(message proto.Message) {
	p.agent.WriteMsg(message)
}

func (p *Player) Stop() {
	// 避免重复事件
	if p.agent != nil {
		p.agent.SetUserData(nil)
		p.agent.Close()
	}

	p.TaskHandler.Stop()
}

func (p *Player) GetUser() (models.User, bool) {
	userData := p.agent.UserData()
	if userData == nil {
		return models.User{}, false
	}
	return userData.(models.User), true
}

func (p *Player) initAvatar() {
	objects, err := bucket.GetOSSClient().GetObjects("avatar/", 10)
	if err != nil {
		fmt.Printf("获取OSS对象列表失败: %v\n", err)
		return
	}
	pathName := objects[utils.RandByArray(objects)].Key
	_, err = bucket.GetOSSClient().CopyObject(pathName, strconv.FormatInt(p.PlayerId, 10))
	if err == nil {
		p.PlayerInfo.AvatarSuffix = filepath.Ext(pathName)
	}
}

func (p *Player) ModifyAvatarSuffix(avatar string) message.Result {
	response := p.SendTask(func() message.Result {
		return p.doModifyAvatarSuffix(avatar)
	})

	if _, ok := response.(error); ok {
		return message.Result_Fail
	}
	return response.(message.Result)
}

func (p *Player) doModifyAvatarSuffix(avatar string) message.Result {
	p.PlayerInfo.AvatarSuffix = "." + avatar
	return message.Result_Success
}
