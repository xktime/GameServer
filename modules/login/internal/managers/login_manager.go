package managers

import (
	"context"
	"gameserver/common/msg/message"
	actor_manager "gameserver/core/actor"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game"
	"gameserver/modules/login/internal/processor"
)

type LoginManager struct {
	actor_manager.ActorMessageHandler `bson:"-"`
}

func (m *LoginManager) HandleLogin(msg *message.C2S_Login, agent gate.Agent) {
	loginProcessor := getLoginProcessor(msg.LoginType)
	if loginProcessor == nil {
		log.Error("loginProcessor is nil")
		return
	}
	loginResp := loginProcessor.ReqLogin(context.Background(), msg)
	log.Debug("loginResp %v", loginResp)
	result := &message.S2C_Login{}
	defer agent.WriteMsg(result)
	// todo 登录失败需要关闭agent
	if loginResp.ErrCode != 0 {
		log.Error("login failed %v", loginResp)
		result.LoginResult = -1
		return
	}
	result.LoginResult = 1
	game.External.UserManager.DirectCaller.UserLogin(agent, loginResp.Openid, msg.ServerId)
}

func getLoginProcessor(loginType message.LoginType) processor.BaseLoginProcessor {
	switch loginType {
	case message.LoginType_DouYin:
		return processor.NewDouyinLoginProcessor()
	case message.LoginType_WeChat:
		return processor.NewWechatLoginProcessor()
	default:
		return nil
	}
}
