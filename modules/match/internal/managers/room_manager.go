package managers

import (
	"gameserver/common/base/actor"
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game"
	"sync"
)

// RoomManager 使用TaskHandler实现，确保房间操作按顺序执行
type RoomManager struct {
	actor.BaseActor
}

var (
	roomManager     *RoomManager
	roomManagerOnce sync.Once
)

func GetRoomManager() *RoomManager {
	roomManagerOnce.Do(func() {
		roomManager = actor.RegisterActor[*RoomManager](actor.Match, "room")
	})
	return roomManager
}

// Init 初始化RoomManager
func (m *RoomManager) Init(args ...any) {
	// 初始化逻辑
}

// Stop 停止RoomManager
func (m *RoomManager) Stop() {
	m.RemoveActor(m)
}

// HandleRecordOperate 处理游戏操作记录 - 异步执行
func (r *RoomManager) HandleRecordOperate(msg *message.C2S_RecordGameOperate, agent gate.Agent) (string, *message.S2C_RecordGameOperate) {
	result := r.SendTask(func() (string, *message.S2C_RecordGameOperate) {
		return r.doHandleRecordOperate(msg, agent)
	})

	if err, ok := result.(error); ok {
		log.Error("处理游戏操作记录失败: %v", err)
		return "", nil
	}

	if results, ok := result.([]interface{}); ok && len(results) >= 2 {
		if roomId, ok := results[0].(string); ok {
			if recordOperateResp, ok := results[1].(*message.S2C_RecordGameOperate); ok {
				return roomId, recordOperateResp
			}
		}
	}
	return "", nil
}

// doHandleRecordOperate 处理游戏操作记录的同步实现
func (r *RoomManager) doHandleRecordOperate(msg *message.C2S_RecordGameOperate, agent gate.Agent) (string, *message.S2C_RecordGameOperate) {
	playerId := agent.UserData().(models.User).PlayerId
	team := game.External.TeamManager.GetTeamByPlayerId(playerId)
	if team == nil {
		log.Error("玩家 %d 没有队伍", playerId)
		return "", &message.S2C_RecordGameOperate{
			OperateInfo: "",
		}
	}
	roomId := team.RoomId
	if roomId != msg.RoomId {
		log.Error("队伍 %d 的房间ID不匹配", team.TeamId)
		return "", &message.S2C_RecordGameOperate{
			OperateInfo: "",
		}
	}
	return roomId, &message.S2C_RecordGameOperate{
		OperateInfo: msg.OperateInfo,
	}
}
