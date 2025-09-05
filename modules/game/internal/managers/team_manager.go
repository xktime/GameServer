package managers

import (
	"gameserver/common/base/actor"
	"gameserver/core/log"
	"gameserver/modules/game/internal/managers/team"
	"sync"

	"google.golang.org/protobuf/proto"
)

// TeamManager 使用TaskHandler实现，确保队伍操作按顺序执行
type TeamManager struct {
	*actor.TaskHandler
}

var (
	teamManager     *TeamManager
	teamManagerOnce sync.Once
)

func GetTeamManager() *TeamManager {
	teamManagerOnce.Do(func() {
		teamManager = &TeamManager{}
		teamManager.Init()
	})
	return teamManager
}

// Init 初始化TeamManager
func (m *TeamManager) Init() {
	// 初始化TaskHandler
	m.TaskHandler = actor.InitTaskHandler(actor.Team, "1", m)
	m.TaskHandler.Start()
}

// Stop 停止TeamManager
func (m *TeamManager) Stop() {
	m.TaskHandler.Stop()
}

// GetTeamByPlayerId 通过玩家ID获取队伍 - 异步执行
func (t *TeamManager) GetTeamByPlayerId(playerId int64) *team.Team {
	response := t.SendTask(func() *actor.Response {
		team := t.doGetTeamByPlayerId(playerId)
		return &actor.Response{
			Result: []interface{}{team},
		}
	})

	if response != nil && len(response.Result) > 0 {
		if team, ok := response.Result[0].(*team.Team); ok {
			return team
		}
	}
	return nil
}

// doGetTeamByPlayerId 通过玩家ID获取队伍的同步实现
func (t *TeamManager) doGetTeamByPlayerId(playerId int64) *team.Team {
	player := GetUserManager().GetPlayer(playerId)
	if player == nil {
		return nil
	}
	teamInfo, ok := actor.GetActor[team.Team](actor.Team, player.TeamId)
	if !ok {
		return nil
	}
	return teamInfo
}

// JoinRoom 加入房间 - 异步执行
func (t *TeamManager) JoinRoom(playerId int64, roomId int64) {
	t.SendTask(func() *actor.Response {
		t.doJoinRoom(playerId, roomId)
		return nil
	})
}

// doJoinRoom 加入房间的同步实现
func (t *TeamManager) doJoinRoom(playerId int64, roomId int64) {
	player := GetUserManager().GetPlayer(playerId)
	if player == nil {
		return
	}
	teamInfo, ok := actor.GetActor[team.Team](actor.Team, player.TeamId)
	if !ok {
		return
	}
	teamInfo.JoinRoom(roomId)
}

// LeaveRoom 离开房间 - 异步执行
func (t *TeamManager) LeaveRoom(teamId int64) {
	t.SendTask(func() *actor.Response {
		t.doLeaveRoom(teamId)
		return nil
	})
}

// doLeaveRoom 离开房间的同步实现
func (t *TeamManager) doLeaveRoom(teamId int64) {
	team, ok := actor.GetActor[team.Team](actor.Team, teamId)
	if !ok {
		return
	}
	team.LeaveRoom()
}

// SendMessage 发送消息给队伍 - 异步执行
func (t *TeamManager) SendMessage(teamId int64, msg proto.Message) {
	t.SendTask(func() *actor.Response {
		t.doSendMessage(teamId, msg)
		return nil
	})
}

// doSendMessage 发送消息给队伍的同步实现
func (t *TeamManager) doSendMessage(teamId int64, msg proto.Message) {
	team, ok := actor.GetActor[team.Team](actor.Team, teamId)
	if !ok {
		return
	}
	for _, member := range team.TeamMembers {
		p := GetUserManager().GetPlayer(member)
		if p == nil {
			log.Debug("玩家 %d 不在线", member)
			continue
		}
		p.SendToClient(msg)
	}
}
