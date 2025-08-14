package handlers

import (
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game/internal/managers"
)

// C2S_GetPlayerInfoHandler 处理C2S_GetPlayerInfo消息
func C2S_GetPlayerInfoHandler(args []interface{}) {
	if len(args) < 2 {
		log.Error("C2S_GetPlayerInfoHandler: 参数不足")
		return
	}

	msg, ok := args[0].(*message.C2S_GetPlayerInfo)
	if !ok {
		log.Error("C2S_GetPlayerInfoHandler: 消息类型错误")
		return
	}

	agent, ok := args[1].(gate.Agent)
	if !ok {
		log.Error("C2S_GetPlayerInfoHandler: Agent类型错误")
		return
	}

	log.Debug("收到C2S_GetPlayerInfo消息: %v, agent: %v", msg, agent)
	playerId := agent.UserData().(models.User).PlayerId
	player := managers.GetUserManager().GetPlayer(playerId)
	playerInfo := player.ToPlayerInfo()
	player.SendToClient(&message.S2C_GetPlayerInfo{
		PlayerInfo: playerInfo,
	})
}
