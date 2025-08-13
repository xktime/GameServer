package handlers

import (
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/rank/internal/managers"
)

// C2S_GetMyRankHandler 处理C2S_GetMyRank消息
func C2S_GetMyRankHandler(args []interface{}) {
	if len(args) < 2 {
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

	// 获取排行榜管理器
	rankManager := managers.GetRankManager()

	playerId := agent.UserData().(models.User).PlayerId

	// 获取我的排名
	rankManager.HandleGetMyRank(playerId, msg.RankType)

}
