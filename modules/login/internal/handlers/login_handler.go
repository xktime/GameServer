package handlers

import (
	"context"
	"gameserver/common/msg/message"
	"gameserver/modules/game"
	"gameserver/modules/login/internal/processor"

	"gameserver/core/gate"
	"gameserver/core/log"
)

func HandleLogin(args []interface{}) {
	m := args[0].(*message.C2S_Login)
	agent := args[1].(gate.Agent)
	DoHandleLogin(m, agent)
}

// todo 默认都要放actor执行？不用的自己go出去，静态方法需要放actor吗？
// todo gate->login(处理登录校验逻辑)->user(处理登录数据初始化逻辑)
func DoHandleLogin(m *message.C2S_Login, agent gate.Agent) {
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

	game.UserManager.DoLogin(agent, loginResp.Openid, m.ServerId)
}

func getLoginProcessor(loginType message.LoginType) processor.BaseLoginProcessor {
	switch loginType {
	case message.LoginType_DouYin:
		return processor.NewDouyinLoginProcessor()
	default:
		return nil
	}
}
