package handlers

import (
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
)

// C2S_ModifyNameHandler 处理C2S_ModifyName消息
func C2S_ModifyNameHandler(users UserManager, args []interface{}) {
	if len(args) < 3 {
		log.Error("C2S_ModifyNameHandler: 参数不足")
		return
	}

	msg, ok := args[0].(*message.C2S_ModifyName)
	if !ok {
		log.Error("C2S_ModifyNameHandler: 消息类型错误")
		return
	}

	agent, ok := args[1].(gate.Agent)
	if !ok {
		log.Error("C2S_ModifyNameHandler: Agent类型错误")
		return
	}

	log.Debug("收到C2S_ModifyName消息: %v, agent: %v", msg, agent)
	playerId := agent.UserData().(models.User).PlayerId
	resultMsg := &message.S2C_ModifyName{
		Result: message.Result_Success,
	}
	defer agent.WriteMsgWithSeq(resultMsg, args[2].(uint32))
	result := users.CheckName(msg.Name)
	if result != message.Result_Success {
		resultMsg.Result = result
		resultMsg.Name = ""
		return
	}
	result, name := users.ModifyName(playerId, msg.Name)
	resultMsg.Result = result
	resultMsg.Name = name
}
