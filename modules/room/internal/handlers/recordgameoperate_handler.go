package handlers

import (
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/room/internal/managers"
)

func C2S_RecordGameOperateHandler(args []interface{}) {
	if len(args) < 3 {
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
	user, ok := agent.UserData().(models.User)
	if !ok {
		log.Error("C2S_RecordGameOperateHandler: UserData类型错误")
		return
	}
	seq, ok := args[2].(uint32)
	if !ok {
		log.Error("C2S_RecordGameOperateHandler: Seq类型错误")
		return
	}

	response := managers.GetRoomManager().HandleRecordOperate(user.PlayerId, msg.RoomId, msg.OperateInfo)
	agent.WriteMsgWithSeq(response, seq)
}
