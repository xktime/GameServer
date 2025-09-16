package handlers

import (
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/match/internal/managers"
	"gameserver/modules/match/internal/managers/room"
)

// C2S_RecordGameOperateHandler 处理C2S_RecordGameOperate消息
func C2S_RecordGameOperateHandler(args []interface{}) {
	if len(args) < 2 {
		log.Error("C2S_RecordGameOperateHandler: 参数不足")
		return
	}

	msg, ok := args[0].(*message.C2S_RecordGameOperate)
	if !ok {
		log.Error("C2S_RecordGameOperateHandler: 消息类型错误")
		return
	}

	agent, ok := args[1].(gate.Agent)
	if !ok {
		log.Error("C2S_RecordGameOperateHandler: Agent类型错误")
		return
	}

	log.Debug("收到C2S_RecordGameOperate消息: %v, agent: %v", msg, agent)
	playerId := agent.UserData().(models.User).PlayerId
	roomId, response := managers.GetRoomManager().HandleRecordOperate(msg, agent)
	if response != nil {
		room.SendRoomMessageExceptSelf(roomId, &message.S2C_RecordGameOperate{
			OperateInfo: msg.OperateInfo,
		}, playerId)
		agent.WriteMsgWithSeq(response, args[2].(uint32))
	}
}
