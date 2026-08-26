package handlers

import (
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
)

// C2S_ChanllengeByCodeHandler 处理C2S_ChanllengeByCode消息
func C2S_ChanllengeByCodeHandler(manager RankManager, args []interface{}) {
	if len(args) < 3 {
		log.Error("C2S_ChanllengeByCodeHandler: 参数不足")
		return
	}

	msg, ok := args[0].(*message.C2S_ChanllengeByCode)
	if !ok {
		log.Error("C2S_ChanllengeByCodeHandler: 消息类型错误")
		return
	}

	agent, ok := args[1].(gate.Agent)
	if !ok {
		log.Error("C2S_ChanllengeByCodeHandler: Agent类型错误")
		return
	}

	seq, ok := args[2].(uint32)
	if !ok {
		log.Error("C2S_ChanllengeByCodeHandler: Seq类型错误")
		return
	}

	log.Debug("收到C2S_ChanllengeByCode消息: %v, agent: %v, seq: %v", msg, agent, seq)
	response := manager.ChanllengeByCode(msg)
	agent.WriteMsgWithSeq(response, seq)
}
