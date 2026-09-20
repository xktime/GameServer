package managers

import (
	"context"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/login/internal/models"
	"gameserver/modules/login/internal/processor"
	"time"

	"google.golang.org/protobuf/proto"
)

// LoginManager 将认证任务提交到有限容量的 Worker Pool。
type LoginManager struct {
	actor.BaseActor
	users    UserLoginService
	authPool *authWorkerPool
	attempts LoginAttemptTracker
}

type UserLoginService interface {
	UserLogin(gate.Agent, string, int32, message.LoginType) *message.S2C_Login
}

// LoginAttemptTracker 为异步登录完成回调提供连接代次校验；
// 生产环境由 ConnectManager 实现。
type LoginAttemptTracker interface {
	BeginLogin(gate.ConnectionID) (LoginAttempt, error)
	WithCurrentLogin(LoginAttempt, func()) bool
	UpdateHeartbeat(gate.ConnectionID)
}

// LoginManagerOptions 配置容量有限的认证工作池。
// 零值表示使用生产环境默认配置。
type LoginManagerOptions struct {
	Authenticator processor.Authenticator
	Workers       int
	QueueSize     int
	Timeout       time.Duration
	Attempts      LoginAttemptTracker
}

func NewLoginManager(ctx context.Context, scope *actor.Scope, users UserLoginService, options ...LoginManagerOptions) (*LoginManager, error) {
	if users == nil {
		return nil, fmt.Errorf("login: UserLoginService is nil")
	}
	config := LoginManagerOptions{
		Authenticator: processor.NewPlatformAuthenticator(),
		Workers:       DefaultAuthWorkers,
		QueueSize:     DefaultAuthQueueSize,
		Timeout:       DefaultAuthTimeout,
	}
	if len(options) > 0 {
		provided := options[0]
		if provided.Authenticator != nil {
			config.Authenticator = provided.Authenticator
		}
		if provided.Workers != 0 {
			config.Workers = provided.Workers
		}
		if provided.QueueSize != 0 {
			config.QueueSize = provided.QueueSize
		}
		if provided.Timeout != 0 {
			config.Timeout = provided.Timeout
		}
		config.Attempts = provided.Attempts
	}
	authPool, err := newAuthWorkerPool(config.Authenticator, config.Workers, config.QueueSize, config.Timeout)
	if err != nil {
		return nil, err
	}
	definition, err := actor.Define(scope, actor.Login, func(context.Context, string) (*LoginManager, error) {
		return &LoginManager{
			users:    users,
			authPool: authPool,
			attempts: config.Attempts,
		}, nil
	})
	if err != nil {
		authPool.close()
		return nil, err
	}
	manager, err := definition.GetOrCreate(ctx, "singleton")
	if err != nil {
		authPool.close()
		return nil, err
	}
	return manager, nil
}

// SubmitLogin 将平台认证任务提交到容量有限的工作池后立即返回，
// 无需等待外部平台响应。工作协程完成后使用原始请求序列号写回响应。
func (m *LoginManager) SubmitLogin(msg *message.C2S_Login, agent gate.ConnectionAgent, sequence uint32) error {
	if msg == nil {
		return fmt.Errorf("login: request is nil")
	}
	if agent == nil {
		return fmt.Errorf("login: connection agent is nil")
	}
	if m.authPool == nil {
		return ErrAuthPoolClosed
	}
	attempt := LoginAttempt{}
	if m.attempts != nil {
		var err error
		attempt, err = m.attempts.BeginLogin(agent.ConnectionID())
		if err != nil {
			return err
		}
	}
	// 复制请求，避免认证工作协程与调用方共享可变消息对象。
	request := proto.Clone(msg).(*message.C2S_Login)
	task := authTask{
		agent:    agent,
		request:  request,
		sequence: sequence,
		attempt:  attempt,
	}
	task.complete = func(response *models.LoginResponse) {
		m.completeLogin(task, response)
	}
	return m.authPool.submit(task)
}

// Close 在已入队请求处理完毕后停止工作协程，
// 由登录模块在关闭时调用。
func (m *LoginManager) Close() {
	if m.authPool != nil {
		m.authPool.close()
	}
}

// completeLogin 在创建会话前及回包前校验请求代次，丢弃已过期的认证结果。
func (m *LoginManager) completeLogin(task authTask, loginResp *models.LoginResponse) {
	complete := func() {
		response := m.completeLoginResponse(task.agent, task.request, loginResp)
		task.agent.WriteMsgWithSeq(response, task.sequence)
		if response.LoginResult != 1 {
			task.agent.Close()
			return
		}
		if m.attempts != nil {
			m.attempts.UpdateHeartbeat(task.agent.ConnectionID())
		}
	}
	if m.attempts == nil {
		complete()
		return
	}
	if !m.attempts.WithCurrentLogin(task.attempt, complete) {
		return
	}
}

func (m *LoginManager) completeLoginResponse(agent gate.Agent, msg *message.C2S_Login, loginResp *models.LoginResponse) *message.S2C_Login {
	if loginResp == nil {
		log.Error("login response is nil")
		return &message.S2C_Login{LoginResult: -1}
	}
	log.Debug("loginResp %v", loginResp)
	if loginResp.ErrCode != 0 {
		log.Error("login failed %v", loginResp)
		return &message.S2C_Login{
			LoginResult: -1,
		}
	}
	response := m.users.UserLogin(agent, loginResp.Openid, msg.ServerId, msg.LoginType)
	if response == nil {
		return &message.S2C_Login{LoginResult: -1}
	}
	return response
}
