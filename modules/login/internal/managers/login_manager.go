package managers

import (
	"context"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game"
	"gameserver/modules/login/internal/processor"
	"sync"
)

// LoginManager 使用TaskHandler实现，确保登录操作按顺序执行
type LoginManager struct {
	actor.BaseActor
}

var (
	loginManager     *LoginManager
	loginManagerOnce sync.Once
)

func GetLoginManager() *LoginManager {
	loginManagerOnce.Do(func() {
		loginManager = actor.RegisterActor[*LoginManager](actor.Login, "1")
	})
	return loginManager
}

// Init 初始化LoginManager
func (m *LoginManager) Init(args ...any) {
	// 初始化逻辑
}

// Stop 停止LoginManager
func (m *LoginManager) Stop() {
	m.RemoveActor(m)
}

// HandleLogin 处理登录请求 - 异步执行
func (m *LoginManager) HandleLogin(msg *message.C2S_Login, agent gate.Agent) *message.S2C_Login {
	response := m.SendTask(func() *message.S2C_Login {
		return m.doHandleLogin(msg, agent)
	})
	return response.(*message.S2C_Login)
}

// doHandleLogin 处理登录请求的同步实现
func (m *LoginManager) doHandleLogin(msg *message.C2S_Login, agent gate.Agent) *message.S2C_Login {
	loginProcessor := getLoginProcessor(msg.LoginType)
	if loginProcessor == nil {
		log.Error("loginProcessor is nil")
		return &message.S2C_Login{
			LoginResult: -1,
		}
	}
	loginResp := loginProcessor.ReqLogin(agent, context.Background(), msg)
	log.Debug("loginResp %v", loginResp)
	if loginResp.ErrCode != 0 {
		log.Error("login failed %v", loginResp)
		return &message.S2C_Login{
			LoginResult: -1,
		}
	}
	return game.External.UserManager.UserLogin(agent, loginResp.Openid, msg.ServerId, msg.LoginType)
}

func getLoginProcessor(loginType message.LoginType) processor.BaseLoginProcessor {
	switch loginType {
	case message.LoginType_DouYin:
		return processor.NewDouyinLoginProcessor()
	case message.LoginType_WeChat:
		return processor.NewWechatLoginProcessor()
	default:
		return processor.NewDefaultLoginProcessor()
	}
}
