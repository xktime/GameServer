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
func (r *RoomManager) HandleRecordOperate(msg *message.C2S_RecordGameOperate, agent gate.Agent) (int64, *message.S2C_RecordGameOperate) {
	response := r.SendTask(func() *actor.Response {
		teamId, recordOperateResp := r.doHandleRecordOperate(msg, agent)
		return &actor.Response{
			Result: []interface{}{teamId, recordOperateResp},
		}
	})
	if response != nil && len(response.Result) > 0 {
		if teamId, ok := response.Result[0].(int64); ok {
			if recordOperateResp, ok := response.Result[1].(*message.S2C_RecordGameOperate); ok {
				return teamId, recordOperateResp
			}
		}
	}
	return 0, nil
}

// doHandleRecordOperate 处理游戏操作记录的同步实现
func (r *RoomManager) doHandleRecordOperate(msg *message.C2S_RecordGameOperate, agent gate.Agent) (int64, *message.S2C_RecordGameOperate) {
	playerId := agent.UserData().(models.User).PlayerId
	team := game.External.TeamManager.GetTeamByPlayerId(playerId)
	if team == nil {
		log.Error("玩家 %d 没有队伍", playerId)
		return 0, &message.S2C_RecordGameOperate{
			OperateInfo: "",
		}
	}
	roomId := team.RoomId
	if roomId != msg.RoomId {
		log.Error("队伍 %d 的房间ID不匹配", team.TeamId)
		return 0, &message.S2C_RecordGameOperate{
			OperateInfo: "",
		}
	}
	return roomId, &message.S2C_RecordGameOperate{
		OperateInfo: msg.OperateInfo,
	}
}
