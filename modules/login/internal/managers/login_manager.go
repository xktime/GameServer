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
	if loginResp.ErrCode != 0 {
		log.Error("login failed %v", loginResp)
		agent.WriteMsg(&message.S2C_Login{
			LoginResult: -1,
		})
		agent.Close()
		return
	}
	game.External.UserManager.UserLogin(agent, loginResp.Openid, msg.ServerId, msg.LoginType)
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
