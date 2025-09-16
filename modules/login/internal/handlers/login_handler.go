package handlers

import (
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/login/internal/managers"
)

// C2S_LoginHandler 处理C2S_Login消息
func C2S_LoginHandler(args []interface{}) {
	if len(args) < 3 {
		log.Error("C2S_LoginHandler: 参数不足")
		return
	}

	msg, ok := args[0].(*message.C2S_Login)
	if !ok {
		log.Error("C2S_LoginHandler: 消息类型错误")
		return
	}

	agent, ok := args[1].(gate.Agent)
	if !ok {
		log.Error("C2S_LoginHandler: Agent类型错误")
		return
	}

	log.Debug("收到C2S_Login消息: %v, agent: %v", msg, agent)
	managers.GetConnectManager().UpdateHeartbeat(agent)
	loginResp := managers.GetLoginManager().HandleLogin(msg, agent)
	if loginResp == nil {
		log.Error("C2S_LoginHandler: loginResp is nil: %v", loginResp)
		return
	}
	agent.WriteMsgWithSeq(loginResp, args[2].(uint32))
	if loginResp.LoginResult != 1 {
		agent.Close()
		return
	}
}
