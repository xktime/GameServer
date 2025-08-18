package handlers

import (
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/login/internal/managers"
)

// C2S_HeartHandler 处理C2S_Heart消息
func C2S_HeartHandler(args []interface{}) {
	if len(args) < 2 {
		log.Error("C2S_HeartHandler: 参数不足")
		return
	}

	msg, ok := args[0].(*message.C2S_Heart)
	if !ok {
		log.Error("C2S_HeartHandler: 消息类型错误")
		return
	}

	agent, ok := args[1].(gate.Agent)
	if !ok {
		log.Error("C2S_HeartHandler: Agent类型错误")
		return
	}

	// 更新客户端心跳
	managers.GetConnectManager().UpdateHeartbeat(agent)

	log.Debug("收到C2S_Heart消息: %v, agent: %v", msg, agent)
}
