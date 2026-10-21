package managers

import (
	"context"
	"errors"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game/playersession"
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
	UserLogin(*playersession.Attempt, string, int32, message.LoginType) error
}

// LoginAttemptTracker 登记登录尝试并刷新连接心跳；
// 生产环境由 ConnectManager 实现。
type LoginAttemptTracker interface {
	BeginLogin(gate.ConnectionID, uint32) (*playersession.Attempt, error)
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
	if config.Attempts == nil {
		return nil, fmt.Errorf("login: LoginAttemptTracker is nil")
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
	attempt, err := m.attempts.BeginLogin(agent.ConnectionID(), sequence)
	if err != nil {
		if errors.Is(err, playersession.ErrAuthenticated) {
			agent.WriteMsgWithSeq(&message.S2C_Login{LoginResult: -1}, sequence)
		}
		return err
	}
	// 复制请求，避免认证工作协程与调用方共享可变消息对象。
	request := proto.Clone(msg).(*message.C2S_Login)
	task := authTask{
		agent:   agent,
		request: request,
		attempt: attempt,
	}
	task.complete = func(response *models.LoginResponse) {
		m.completeLogin(task, response)
	}
	if err := m.authPool.submit(task); err != nil {
		attempt.Fail()
		return err
	}
	return nil
}

// Close 在已入队请求处理完毕后停止工作协程，
// 由登录模块在关闭时调用。
func (m *LoginManager) Close() {
	if m.authPool != nil {
		m.authPool.close()
	}
}

// completeLogin never occupies ConnectManager's Actor. Player owns the success
// reply in its commit task; the Attempt owns any failure reply and close.
func (m *LoginManager) completeLogin(task authTask, loginResp *models.LoginResponse) {
	if !task.attempt.IsCurrent("") {
		return
	}
	if loginResp == nil || loginResp.ErrCode != 0 {
		task.attempt.Fail()
		return
	}
	if err := m.users.UserLogin(task.attempt, loginResp.Openid, task.request.ServerId, task.request.LoginType); err != nil {
		if !errors.Is(err, playersession.ErrStaleAttempt) {
			log.Error("登录准备失败: connection=%d server=%d error=%v", task.agent.ConnectionID(), task.request.ServerId, err)
			task.attempt.Fail()
		}
		return
	}
	m.attempts.UpdateHeartbeat(task.agent.ConnectionID())
}
