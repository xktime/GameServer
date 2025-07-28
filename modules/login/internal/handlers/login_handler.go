package handlers

import (
	"context"
	"gameserver/common/msg/message"
	"gameserver/modules/login/internal/processor"
	"gameserver/modules/login/managers"

	"gameserver/core/gate"
	"gameserver/core/log"
)

// todollw 直接传参进来就行了，没必要在里面再处理一遍
func HandleLogin(args []interface{}) {
	m := args[0].(*message.C2S_Login)
	// // 消息的发送者
	agent := args[1].(gate.Agent)
	loginProcessor := getLoginProcessor(m.LoginType)
	if loginProcessor == nil {
		log.Error("loginProcessor is nil")
		return
	}
	loginResp := loginProcessor.ReqLogin(context.Background(), m)
	log.Debug("loginResp %v", loginResp)
	result := &message.S2C_Login{}
	defer agent.WriteMsg(result)
	if loginResp.ErrCode != 0 {
		log.Error("login failed %v", loginResp)
		result.LoginResult = -1
		return
	}
	result.LoginResult = 0

	managers.GetLoginManager().DoLogin(agent, loginResp.Openid, m.ServerId)
}

func getLoginProcessor(loginType message.LoginType) processor.BaseLoginProcessor {
	switch loginType {
	case message.LoginType_DouYin:
		return processor.NewDouyinLoginProcessor()
	default:
		return nil
	}
}
