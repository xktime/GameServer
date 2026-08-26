package managers

import (
	"context"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/login/internal/processor"
)

// LoginManager 的请求由绑定的 Actor 队列串行处理。
type LoginManager struct {
	actor.BaseActor
	users UserLoginService
}

type UserLoginService interface {
	UserLogin(gate.Agent, string, int32, message.LoginType) *message.S2C_Login
}

func NewLoginManager(ctx context.Context, scope *actor.Scope, users UserLoginService) (*LoginManager, error) {
	if users == nil {
		return nil, fmt.Errorf("login: UserLoginService is nil")
	}
	definition, err := actor.Define(scope, actor.Login, func(context.Context, string) (*LoginManager, error) {
		return &LoginManager{users: users}, nil
	})
	if err != nil {
		return nil, err
	}
	return definition.GetOrCreate(ctx, "singleton")
}

// HandleLogin 同步等待串行化的登录结果。
func (m *LoginManager) HandleLogin(msg *message.C2S_Login, agent gate.Agent) *message.S2C_Login {
	response, err := actor.Call(context.Background(), m.Ref(), func(execution actor.Context) (*message.S2C_Login, error) {
		return m.doHandleLogin(execution, msg, agent), nil
	})
	if err != nil {
		log.Error("处理登录失败: %v", err)
		return nil
	}
	return response
}

// doHandleLogin 在 LoginManager Actor 内执行登录流程。
func (m *LoginManager) doHandleLogin(ctx context.Context, msg *message.C2S_Login, agent gate.Agent) *message.S2C_Login {
	loginProcessor := getLoginProcessor(msg.LoginType)
	if loginProcessor == nil {
		log.Error("loginProcessor is nil")
		return &message.S2C_Login{
			LoginResult: -1,
		}
	}
	loginResp := loginProcessor.ReqLogin(agent, ctx, msg)
	log.Debug("loginResp %v", loginResp)
	if loginResp.ErrCode != 0 {
		log.Error("login failed %v", loginResp)
		return &message.S2C_Login{
			LoginResult: -1,
		}
	}
	return m.users.UserLogin(agent, loginResp.Openid, msg.ServerId, msg.LoginType)
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
