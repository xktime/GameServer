package managers

import (
	"gameserver/common/models"
	"gameserver/common/msg/message"
	actor_manager "gameserver/core/actor"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game"
	"gameserver/modules/match/internal/managers/room"
)

type RoomManager struct {
	actor_manager.ActorMessageHandler `bson:"-"`
}

func (r *RoomManager) HandleRecordOperate(msg *message.C2S_RecordGameOperate, agent gate.Agent) {
	playerId := agent.UserData().(models.User).PlayerId
	team := game.External.TeamManager.DirectCaller.GetTeamByPlayerId(playerId)
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
