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
	*actor.TaskHandler
}

var (
	loginManager     *LoginManager
	loginManagerOnce sync.Once
)

func GetLoginManager() *LoginManager {
	loginManagerOnce.Do(func() {
		loginManager = &LoginManager{}
		loginManager.Init()
	})
	return loginManager
}

// Init 初始化LoginManager
func (m *LoginManager) Init() {
	// 初始化TaskHandler
	m.TaskHandler = actor.InitTaskHandler(actor.Login, "1", m)
	m.TaskHandler.Start()
}

// Stop 停止LoginManager
func (m *LoginManager) Stop() {
	m.TaskHandler.Stop()
}

// HandleLogin 处理登录请求 - 异步执行
func (m *LoginManager) HandleLogin(msg *message.C2S_Login, agent gate.Agent) {
	m.SendTask(func() *actor.Response {
		m.doHandleLogin(msg, agent)
		return nil
	})
}

// doHandleLogin 处理登录请求的同步实现
func (m *LoginManager) doHandleLogin(msg *message.C2S_Login, agent gate.Agent) {
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
