package handlers

import (
	"gameserver/common/msg/message"
	"gameserver/common/session"
	"gameserver/core/gate"
	"gameserver/core/log"

	"google.golang.org/protobuf/proto"
)

type MatchManager interface {
	HandleMatch(int64, *message.C2S_StartMatch) (int64, *message.S2C_StartMatch)
	HandleCancelMatch(int64) (int64, *message.S2C_CancelMatch)
}

type TeamMessenger interface {
	SendMessageExceptSelf(int64, proto.Message, int64)
}

// C2S_StartMatchHandler 处理C2S_StartMatch消息
func C2S_StartMatchHandler(manager MatchManager, teams TeamMessenger, args []interface{}) {
	if len(args) < 3 {
		log.Error("C2S_StartMatchHandler: 参数不足")
		return
	}

	msg, ok := args[0].(*message.C2S_StartMatch)
	if !ok {
		log.Error("C2S_StartMatchHandler: 消息类型错误")
		return
	}

	agent, ok := args[1].(gate.Agent)
	if !ok {
		log.Error("C2S_StartMatchHandler: Agent类型错误")
		return
	}

	log.Debug("收到C2S_StartMatch消息: %v, agent: %v", msg, agent)
	playerID, ok := session.PlayerIDFrom(agent.UserData())
	if !ok {
		log.Error("C2S_StartMatchHandler: Player Session 不存在")
		return
	}
	teamId, response := manager.HandleMatch(playerID, msg)
	if response == nil {
		log.Error("C2S_StartMatchHandler: response is nil")
		return
	}
	if response.Result {
		teams.SendMessageExceptSelf(teamId, response, playerID)
	}
	agent.WriteMsgWithSeq(response, args[2].(uint32))
}
