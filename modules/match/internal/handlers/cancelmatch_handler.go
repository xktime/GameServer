package handlers

import (
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/match/internal/managers"
)

// C2S_CancelMatchHandler 处理C2S_CancelMatch消息
func C2S_CancelMatchHandler(args []interface{}) {
	if len(args) < 2 {
		log.Error("C2S_CancelMatchHandler: 参数不足")
		return
	}

	msg, ok := args[0].(*message.C2S_CancelMatch)
	if !ok {
		log.Error("C2S_CancelMatchHandler: 消息类型错误")
		return
	}

	agent, ok := args[1].(gate.Agent)
	if !ok {
		log.Error("C2S_CancelMatchHandler: Agent类型错误")
		return
	}

	log.Debug("收到C2S_CancelMatch消息: %v, agent: %v", msg, agent)
	managers.GetMatchManager().HandleCancelMatch(agent)
}
