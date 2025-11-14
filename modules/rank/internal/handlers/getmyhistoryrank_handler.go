package handlers

import (
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/rank/internal/managers"
)

// C2S_GetMyHistoryRankHandler 处理C2S_GetMyHistoryRank消息
func C2S_GetMyHistoryRankHandler(args []interface{}) {
	if len(args) < 3 {
		log.Error("C2S_GetMyHistoryRankHandler: 参数不足")
		return
	}

	msg, ok := args[0].(*message.C2S_GetMyHistoryRank)
	if !ok {
		log.Error("C2S_GetMyHistoryRankHandler: 消息类型错误")
		return
	}

	agent, ok := args[1].(gate.Agent)
	if !ok {
		log.Error("C2S_GetMyHistoryRankHandler: Agent类型错误")
		return
	}

	seq, ok := args[2].(uint32)
	if !ok {
		log.Error("C2S_GetMyHistoryRankHandler: Seq类型错误")
		return
	}

	log.Debug("收到C2S_GetMyHistoryRank消息: %v, agent: %v, seq: %v", msg, agent, seq)
	playerId := agent.UserData().(models.User).PlayerId
	response := managers.GetRankManager().GetMyHistoryRank(playerId)
	agent.WriteMsgWithSeq(response, seq)
}
