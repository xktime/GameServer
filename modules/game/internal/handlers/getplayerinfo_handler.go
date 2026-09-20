package handlers

import (
	"gameserver/common/msg/message"
	"gameserver/common/session"
	"gameserver/core/gate"
	"gameserver/core/log"
)

// C2S_GetPlayerInfoHandler 处理C2S_GetPlayerInfo消息
func C2S_GetPlayerInfoHandler(users UserManager, args []interface{}) {
	if len(args) < 3 {
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
	var playerInfo *message.PlayerInfo
	defer func() {
		agent.WriteMsgWithSeq(&message.S2C_GetPlayerInfo{
			PlayerInfo: playerInfo,
		}, args[2].(uint32))
	}()

	log.Debug("收到C2S_GetPlayerInfo消息: %v, agent: %v", msg, agent)
	playerID, ok := session.PlayerIDFrom(agent.UserData())
	if !ok {
		log.Error("C2S_GetPlayerInfoHandler: Player Session 不存在")
		return
	}
	if info, found := users.GetPlayerInfo(playerID); found {
		playerInfo = info
	}
}
