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
	actor.BaseActor
}

var (
	teamManager     *TeamManager
	teamManagerOnce sync.Once
)

func GetTeamManager() *TeamManager {
	teamManagerOnce.Do(func() {
		teamManager = actor.RegisterActor[*TeamManager](actor.Team, "1")
	})
	return teamManager
}

// Init 初始化TeamManager
func (m *TeamManager) Init(args ...any) {
	// 初始化逻辑
}

// Stop 停止TeamManager
func (m *TeamManager) Stop() {
	m.RemoveActor(m)
}

// GetTeamByPlayerId 通过玩家ID获取队伍 - 异步执行
func (t *TeamManager) GetTeamByPlayerId(playerId int64) *team.Team {
	result := t.SendTask(func() *team.Team {
		return t.doGetTeamByPlayerId(playerId)
	})

	if err, ok := result.(error); ok {
		log.Error("获取队伍失败: %v", err)
		return nil
	}

	if team, ok := result.(*team.Team); ok {
		return team
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

func (t *TeamManager) SetRoomProjection(teamID int64, roomID string) bool {
	result := t.SendTask(func() bool {
		teamActor, ok := actor.GetActor[team.Team](actor.Team, teamID)
		if !ok {
			return false
		}
		return teamActor.SetRoomProjection(roomID)
	})
	if err, ok := result.(error); ok {
		log.Error("设置队伍 %d 的房间投影失败: %v", teamID, err)
		return false
	}
	applied, ok := result.(bool)
	return ok && applied
}

// SendMessage 发送消息给队伍 - 异步执行
func (t *TeamManager) SendMessage(teamId int64, msg proto.Message) {
	t.SendTaskAsync(func() {
		t.doSendMessage(teamId, msg)
	})
}

func (t *TeamManager) SendMessageExceptSelf(teamId int64, msg proto.Message, selfId int64) {
	t.SendTaskAsync(func() {
		t.doSendMessageExceptSelf(teamId, msg, selfId)
	})
}

func (t *TeamManager) doSendMessageExceptSelf(teamId int64, msg proto.Message, selfId int64) {
	team, ok := actor.GetActor[team.Team](actor.Team, teamId)
	if !ok {
		return
	}
	for _, member := range team.TeamMembers {
		if member == selfId {
			continue
		}
		p := GetUserManager().GetPlayer(member)
		if p == nil {
			log.Debug("玩家 %d 不在线", member)
			continue
		}
		p.SendToClient(msg)
	}
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
