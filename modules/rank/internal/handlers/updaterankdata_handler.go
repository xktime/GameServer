package handlers

import (
	"gameserver/common/msg/message"
	"gameserver/common/session"
	"gameserver/core/gate"
	"gameserver/core/log"
)

// C2S_UpdateRankDataHandler 处理C2S_UpdateRankData消息
func C2S_UpdateRankDataHandler(manager RankManager, args []interface{}) {
	if len(args) < 3 {
		log.Error("C2S_UpdateRankDataHandler: 参数不足")
		return
	}

	msg, ok := args[0].(*message.C2S_UpdateRankData)
	if !ok {
		log.Error("C2S_UpdateRankDataHandler: 消息类型错误")
		return
	}

	agent, ok := args[1].(gate.Agent)
	if !ok {
		log.Error("C2S_UpdateRankDataHandler: Agent类型错误")
		return
	}

	log.Debug("收到C2S_UpdateRankData消息: %v, agent: %v", msg, agent)

	playerID, ok := session.PlayerIDFrom(agent.UserData())
	if !ok {
		log.Error("C2S_UpdateRankDataHandler: Player Session 不存在")
		return
	}
	response := manager.HandleUpdateRankData(playerID, msg)
	if response != nil {
		agent.WriteMsgWithSeq(response, args[2].(uint32))
	}
}
