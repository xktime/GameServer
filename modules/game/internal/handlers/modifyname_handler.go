package handlers

import (
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game/internal/managers"
)

// C2S_ModifyNameHandler 处理C2S_ModifyName消息
func C2S_ModifyNameHandler(args []interface{}) {
	if len(args) < 2 {
		log.Error("C2S_ModifyNameHandler: 参数不足")
		return
	}

	msg, ok := args[0].(*message.C2S_ModifyName)
	if !ok {
		log.Error("C2S_ModifyNameHandler: 消息类型错误")
		return
	}

	agent, ok := args[1].(gate.Agent)
	if !ok {
		log.Error("C2S_ModifyNameHandler: Agent类型错误")
		return
	}

	log.Debug("收到C2S_ModifyName消息: %v, agent: %v", msg, agent)
	userManager := managers.GetUserManager()
	playerId := agent.UserData().(models.User).PlayerId
	p := userManager.GetPlayer(playerId)
	resultMsg := &message.S2C_ModifyName{
		Result: message.Result_Success,
	}
	defer p.SendToClient(resultMsg)
	if p == nil {
		log.Error("C2S_ModifyNameHandler: 玩家不在线")
		resultMsg.Result = message.Result_Fail
		return
	}
	result := userManager.CheckName(msg.Name)
	if result != message.Result_Success {
		resultMsg.Result = result
		return
	}
	resultMsg.Result = userManager.ModifyName(playerId, msg.Name)
	p.SendToClient(resultMsg)
}
