package handlers

import (
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/rank/internal/managers"
)

// C2S_GetRankListHandler 处理C2S_GetRankList消息
func C2S_GetRankListHandler(args []interface{}) {
	if len(args) < 2 {
		log.Error("C2S_GetRankListHandler: 参数不足")
		return
	}

	msg, ok := args[0].(*message.C2S_GetRankList)
	if !ok {
		log.Error("C2S_GetRankListHandler: 消息类型错误")
		return
	}

	agent, ok := args[1].(gate.Agent)
	if !ok {
		log.Error("C2S_GetRankListHandler: Agent类型错误")
		return
	}

	log.Debug("收到C2S_GetRankList消息: %v, agent: %v", msg, agent)

	// 获取排行榜数据
	playerId := agent.UserData().(models.User).PlayerId
	managers.GetRankManager().HandleGetRankList(playerId, msg)
}
