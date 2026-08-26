package handlers

import (
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"time"
)

// C2S_HeartHandler 处理C2S_Heart消息
func C2S_HeartHandler(connections ConnectManager, args []interface{}) {
	if len(args) < 3 {
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
	connections.UpdateHeartbeat(agent)
	agent.WriteMsgWithSeq(&message.S2C_Heart{
		Timestamp: int32(time.Now().Unix()),
	}, args[2].(uint32))
	log.Debug("收到C2S_Heart消息: %v, agent: %v", msg, agent)
}
