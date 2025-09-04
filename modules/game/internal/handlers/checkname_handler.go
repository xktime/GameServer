package handlers

import (
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game/internal/managers"
)

// C2S_CheckNameHandler 处理C2S_CheckName消息
func C2S_CheckNameHandler(args []interface{}) {
	if len(args) < 2 {
		log.Error("C2S_CheckNameHandler: 参数不足")
		return
	}

	msg, ok := args[0].(*message.C2S_CheckName)
	if !ok {
		log.Error("C2S_CheckNameHandler: 消息类型错误")
		return
	}

	agent, ok := args[1].(gate.Agent)
	if !ok {
		log.Error("C2S_CheckNameHandler: Agent类型错误")
		return
	}

	log.Debug("收到C2S_CheckName消息: %v, agent: %v", msg, agent)
	playerName := msg.Name
	result := managers.GetUserManager().CheckName(playerName)
	agent.WriteMsg(&message.S2C_CheckName{
		Result: result,
	})
}
