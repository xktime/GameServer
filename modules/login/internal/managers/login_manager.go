package managers

import (
	"context"
	"gameserver/common/msg/message"
	actor_manager "gameserver/core/actor"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game"
	"gameserver/modules/login/internal/processor"
	"sync"
)

type LoginManager struct {
	actor_manager.ActorMessageHandler
	//	actorMeta actor_manager.ActorMeta[LoginManager]
}

var (
	meta      *actor_manager.ActorMeta[LoginManager]
	loginOnce sync.Once
)

func GetLoginManager() *LoginManager {
	loginOnce.Do(func() {
		meta, _ = actor_manager.Register[LoginManager]("1", actor_manager.Login)

	})
	return meta.Actor
}

func (m *LoginManager) HandleLogin(args []interface{}) {
	msg := args[0].(*message.C2S_Login)
	agent := args[1].(gate.Agent)
	m.DoHandleLogin(msg, agent)
}

func (m *LoginManager) DoHandleLogin(msg *message.C2S_Login, agent gate.Agent) {
	loginProcessor := getLoginProcessor(msg.LoginType)
	if loginProcessor == nil {
		log.Error("loginProcessor is nil")
		return
	}
	loginResp := loginProcessor.ReqLogin(context.Background(), msg)
	log.Debug("loginResp %v", loginResp)
	result := &message.S2C_Login{}
	defer agent.WriteMsg(result)
	if loginResp.ErrCode != 0 {
		log.Error("login failed %v", loginResp)
		result.LoginResult = -1
		return
	}
	result.LoginResult = 0

	game.External.UserManager.DoLoginByActor(agent, loginResp.Openid, msg.ServerId)
}

func (m *LoginManager) DoHandleLoginByActor(msg *message.C2S_Login, agent gate.Agent) {
	meta.AddToActor("DoHandleLogin", []interface{}{msg, agent})
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
