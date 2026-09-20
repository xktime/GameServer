package handlers

import (
	"gameserver/common/msg/message"
	"gameserver/common/session"
	"gameserver/core/gate"
	"gameserver/core/log"
)

// C2S_GetMyRankHandler 处理C2S_GetMyRank消息
func C2S_GetMyRankHandler(manager RankManager, args []interface{}) {
	if len(args) < 3 {
		log.Error("C2S_GetMyRankHandler: 参数不足")
		return
	}

	msg, ok := args[0].(*message.C2S_GetMyRank)
	if !ok {
		log.Error("C2S_GetMyRankHandler: 消息类型错误")
		return
	}

	agent, ok := args[1].(gate.Agent)
	if !ok {
		log.Error("C2S_GetMyRankHandler: Agent类型错误")
		return
	}

	log.Debug("收到C2S_GetMyRank消息: %v, agent: %v", msg, agent)

	playerID, ok := session.PlayerIDFrom(agent.UserData())
	if !ok {
		log.Error("C2S_GetMyRankHandler: Player Session 不存在")
		return
	}

	// 获取我的排名
	response := manager.HandleGetMyRank(playerID, msg.RankType, msg.Season)
	if response != nil {
		agent.WriteMsgWithSeq(response, args[2].(uint32))
	}

}
