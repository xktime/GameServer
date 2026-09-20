package handlers

import (
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
)

type LoginManager interface {
	SubmitLogin(*message.C2S_Login, gate.ConnectionAgent, uint32) error
	Close()
}

type ConnectManager interface {
	Register(gate.ConnectionAgent)
	UpdateHeartbeat(gate.ConnectionID)
	RemoveClient(gate.ConnectionID)
}

// C2S_LoginHandler 处理C2S_Login消息
func C2S_LoginHandler(login LoginManager, args []interface{}) {
	if len(args) < 3 {
		log.Error("C2S_LoginHandler: 参数不足")
		return
	}

	msg, ok := args[0].(*message.C2S_Login)
	if !ok {
		log.Error("C2S_LoginHandler: 消息类型错误")
		return
	}

	agent, ok := args[1].(gate.ConnectionAgent)
	if !ok {
		log.Error("C2S_LoginHandler: ConnectionAgent类型错误")
		return
	}
	sequence, ok := args[2].(uint32)
	if !ok {
		log.Error("C2S_LoginHandler: 序号类型错误")
		return
	}

	log.Debug("收到C2S_Login消息: %v, agent: %v", msg, agent)
	if err := login.SubmitLogin(msg, agent, sequence); err != nil {
		log.Error("C2S_LoginHandler: 提交异步登录失败: %v", err)
		writeLoginFailure(agent, sequence)
	}
}

func writeLoginFailure(agent gate.ConnectionAgent, sequence uint32) {
	agent.WriteMsgWithSeq(&message.S2C_Login{LoginResult: -1}, sequence)
	agent.Close()
}
