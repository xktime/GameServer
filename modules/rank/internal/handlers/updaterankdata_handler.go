package handlers

import (
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/rank/internal/managers"
)

// C2S_UpdateRankDataHandler 处理C2S_UpdateRankData消息
func C2S_UpdateRankDataHandler(args []interface{}) {
	if len(args) < 2 {
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

	playerId := agent.UserData().(models.User).PlayerId
	managers.GetRankManager().HandleUpdateRankData(playerId, msg)
}
