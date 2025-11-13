package team

import (
	"gameserver/common/base/actor"
	"gameserver/common/db/mongodb"
	"gameserver/common/models"
	"gameserver/common/utils"
	"gameserver/core/gate"
	"gameserver/core/log"
	"slices"
)

type Team struct {
	actor.BaseActor `bson:"-"`
	TeamId          int64   `bson:"_id"`
	LeaderId        int64   `bson:"leader_id"`
	TeamMembers     []int64 `bson:"team_members"`
	RoomId          string  `bson:"room_id"`
}

func (t Team) GetPersistId() interface{} {
	return t.TeamId
}

// 初始化队伍
func InitTeam(agent gate.Agent) *Team {
	user := agent.UserData().(models.User)
	playerId := user.PlayerId

	teamId := utils.FlakeId()
	log.Debug("开始初始化队伍，玩家ID: %d, 队伍ID: %d", playerId, teamId)

	return actor.RegisterActor[*Team](actor.Team, teamId, teamId, playerId)
}

func (t *Team) Init(args ...any) {
	if teamId, ok := args[0].(int64); ok {
		t.TeamId = teamId
	}
	if leaderId, ok := args[1].(int64); ok {
		t.LeaderId = leaderId
	}
}

func (t *Team) Stop() {
	t.RemoveActor(t)
}

func (t *Team) JoinTeam(playerId int64) {
	t.SendTaskAsync(func() {
		t.doJoinTeam(playerId)
	})
}

func (t *Team) doJoinTeam(playerId int64) {
	if t.LeaderId == 0 {
		t.LeaderId = playerId
	}

	t.TeamMembers = append(t.TeamMembers, playerId)
	log.Debug("玩家 %d 成功加入队伍 %d，当前成员数量: %d", playerId, t.TeamId, len(t.TeamMembers))
}

func (t *Team) JoinRoom(roomId string) {
	t.SendTaskAsync(func() {
		t.doJoinRoom(roomId)
	})
}

func (t *Team) doJoinRoom(roomId string) {
	t.RoomId = roomId
	log.Debug("队伍 %d 成功加入房间 %s", t.TeamId, roomId)
}

func (t *Team) LeaveRoom() {
	t.SendTaskAsync(func() {
		t.doLeaveRoom()
	})
}

func (t *Team) doLeaveRoom() {
	t.RoomId = ""
	log.Debug("队伍 %d 成功离开房间", t.TeamId)
}

func (t *Team) LeaveTeam(playerId int64) {
	t.SendTaskAsync(func() {
		t.doLeaveTeam(playerId)
	})
}

func (t *Team) doLeaveTeam(playerId int64) {
	log.Debug("玩家 %d 请求离开队伍 %d", playerId, t.TeamId)

	// 检查是否是队长离开
	if t.IsLeader(playerId) {
		t.LeaderId = 0
		log.Debug("队伍 %d 的队长 %d 离开，队长职位空缺", t.TeamId, playerId)
	}

	// 从成员列表中移除
	for i, v := range t.TeamMembers {
		if v == playerId {
			t.TeamMembers = append(t.TeamMembers[:i], t.TeamMembers[i+1:]...)
			log.Debug("从队伍 %d 中移除玩家 %d", t.TeamId, playerId)
			break
		}
	}

	// 检查队伍是否为空
	if len(t.TeamMembers) == 0 {
		log.Debug("队伍 %d 已无成员，停止队伍Actor", t.TeamId)
		t.Stop()
		mongodb.DeleteByID[Team](t.TeamId)
		return
	}

	// 如果队长职位空缺，选择第一个成员作为新队长
	if t.LeaderId == 0 && len(t.TeamMembers) > 0 {
		t.LeaderId = t.TeamMembers[0]
		log.Debug("队伍 %d 选择新队长: %d", t.TeamId, t.LeaderId)
	}

	log.Debug("玩家 %d 离开队伍 %d 完成，剩余成员数量: %d", playerId, t.TeamId, len(t.TeamMembers))
}

// IsMember 检查玩家是否是队伍成员
func (t *Team) IsMember(playerId int64) bool {
	result := t.SendTask(func() bool {
		return t.doIsMember(playerId)
	})

	if err, ok := result.(error); ok {
		log.Error("检查队员失败: %v", err)
		return false
	}

	if typedResult, ok := result.(bool); ok {
		return typedResult
	}
	return false
}

func (t *Team) doIsMember(playerId int64) bool {
	return slices.Contains(t.TeamMembers, playerId)
}

// IsLeader 检查玩家是否是队长
func (t *Team) IsLeader(playerId int64) bool {
	result := t.SendTask(func() bool {
		return t.doIsLeader(playerId)
	})

	if err, ok := result.(error); ok {
		log.Error("检查队长失败: %v", err)
		return false
	}

	if typedResult, ok := result.(bool); ok {
		return typedResult
	}
	return false
}

func (t *Team) doIsLeader(playerId int64) bool {
	return t.LeaderId == playerId
}
