package processor

import (
	"context"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/modules/login/internal/models"
)

// Authenticator 校验平台登录凭证。通过接口隔离具体实现，
// 使登录工作池的测试无需调用外部平台。
type Authenticator interface {
	Authenticate(context.Context, gate.Agent, *message.C2S_Login) *models.LoginResponse
}

// PlatformAuthenticator 将请求分发给已配置的平台处理器，
// 并保持现有登录协议不变。
type PlatformAuthenticator struct{}

func NewPlatformAuthenticator() PlatformAuthenticator {
	return PlatformAuthenticator{}
}

func (PlatformAuthenticator) Authenticate(ctx context.Context, agent gate.Agent, req *message.C2S_Login) *models.LoginResponse {
	if req == nil {
		return &models.LoginResponse{ErrCode: -1, ErrMsg: "login request is nil"}
	}
	loginProcessor := getLoginProcessor(req.LoginType)
	if loginProcessor == nil {
		return &models.LoginResponse{ErrCode: -1, ErrMsg: "login processor is nil"}
	}
	return loginProcessor.ReqLogin(agent, ctx, req)
}

func getLoginProcessor(loginType message.LoginType) BaseLoginProcessor {
	switch loginType {
	case message.LoginType_DouYin:
		return NewDouyinLoginProcessor()
	case message.LoginType_WeChat:
		return NewWechatLoginProcessor()
	default:
		return NewDefaultLoginProcessor()
	}
}
