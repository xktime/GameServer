package handlers

import (
	"gameserver/common/msg/message"
	"gameserver/common/session"
	"gameserver/core/gate"
	"gameserver/core/log"
)

type RoomManager interface {
	HandleRecordOperate(int64, string, string) *message.S2C_RecordGameOperate
}

func C2S_RecordGameOperateHandler(manager RoomManager, args []interface{}) {
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
	playerID, ok := session.PlayerIDFrom(agent.UserData())
	if !ok {
		log.Error("C2S_RecordGameOperateHandler: Player Session 不存在")
		return
	}
	seq, ok := args[2].(uint32)
	if !ok {
		log.Error("C2S_RecordGameOperateHandler: Seq类型错误")
		return
	}

	response := manager.HandleRecordOperate(playerID, msg.RoomId, msg.OperateInfo)
	agent.WriteMsgWithSeq(response, seq)
}
