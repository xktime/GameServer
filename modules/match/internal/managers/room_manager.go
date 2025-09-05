package managers

import (
	"gameserver/common/base/actor"
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game"
	"gameserver/modules/match/internal/managers/room"
	"sync"
)

// RoomManager 使用TaskHandler实现，确保房间操作按顺序执行
type RoomManager struct {
	*actor.TaskHandler
}

var (
	roomManager     *RoomManager
	roomManagerOnce sync.Once
)

func GetRoomManager() *RoomManager {
	roomManagerOnce.Do(func() {
		roomManager = &RoomManager{}
		roomManager.Init()
	})
	return roomManager
}

// Init 初始化RoomManager
func (m *RoomManager) Init() {
	// 初始化TaskHandler
	m.TaskHandler = actor.InitTaskHandler(actor.Match, "room", m)
	m.TaskHandler.Start()
}

// Stop 停止RoomManager
func (m *RoomManager) Stop() {
	m.TaskHandler.Stop()
}

// HandleRecordOperate 处理游戏操作记录 - 异步执行
func (r *RoomManager) HandleRecordOperate(msg *message.C2S_RecordGameOperate, agent gate.Agent) {
	r.SendTask(func() *actor.Response {
		r.doHandleRecordOperate(msg, agent)
		return nil
	})
}

// doHandleRecordOperate 处理游戏操作记录的同步实现
func (r *RoomManager) doHandleRecordOperate(msg *message.C2S_RecordGameOperate, agent gate.Agent) {
	playerId := agent.UserData().(models.User).PlayerId
	team := game.External.TeamManager.GetTeamByPlayerId(playerId)
	if team == nil {
		log.Error("玩家 %d 没有队伍", playerId)
		return
	}
	roomId := team.RoomId
	if roomId != msg.RoomId {
		log.Error("队伍 %d 的房间ID不匹配", team.TeamId)
		return
	}
	room.SendRoomMessage(roomId, &message.S2C_RecordGameOperate{
		OperateInfo: msg.OperateInfo,
	})
}
