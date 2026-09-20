package handlers

import (
	"gameserver/common/msg/message"
	"gameserver/common/session"
	"gameserver/core/gate"
	"gameserver/core/log"
)

// C2S_CancelMatchHandler 处理C2S_CancelMatch消息
func C2S_CancelMatchHandler(manager MatchManager, teams TeamMessenger, args []interface{}) {
	if len(args) < 3 {
		log.Error("C2S_CancelMatchHandler: 参数不足")
		return
	}

	msg, ok := args[0].(*message.C2S_CancelMatch)
	if !ok {
		log.Error("C2S_CancelMatchHandler: 消息类型错误")
		return
	}

	agent, ok := args[1].(gate.Agent)
	if !ok {
		log.Error("C2S_CancelMatchHandler: Agent类型错误")
		return
	}

	log.Debug("收到C2S_CancelMatch消息: %v, agent: %v", msg, agent)
	playerID, ok := session.PlayerIDFrom(agent.UserData())
	if !ok {
		log.Error("C2S_CancelMatchHandler: Player Session 不存在")
		return
	}
	teamId, response := manager.HandleCancelMatch(playerID)
	if response == nil {
		log.Error("C2S_CancelMatchHandler: response is nil")
		return
	}
	if response.Result {
		teams.SendMessageExceptSelf(teamId, response, playerID)
	}
	agent.WriteMsgWithSeq(response, args[2].(uint32))
}
