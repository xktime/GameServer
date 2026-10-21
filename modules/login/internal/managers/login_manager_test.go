package managers

import (
	"context"
	"gameserver/common/base/actor"
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/modules/game/playersession"
	"testing"
	"time"
)

type asyncLoginAgent struct {
	testConnectionAgent
	writes chan loginWrite
}

type loginWrite struct {
	message  any
	sequence uint32
}

func (a *asyncLoginAgent) WriteMsgWithSeq(message any, sequence uint32) {
	a.writes <- loginWrite{message: message, sequence: sequence}
}

type fakeLoginAttempts struct {
	state      *playersession.State
	heartbeats chan gate.ConnectionID
}

func (a *fakeLoginAttempts) BeginLogin(connectionID gate.ConnectionID, sequence uint32) (*playersession.Attempt, error) {
	return a.state.Begin(connectionID, sequence)
}

func (a *fakeLoginAttempts) UpdateHeartbeat(connectionID gate.ConnectionID) {
	a.heartbeats <- connectionID
}

type fakeUserLoginService struct{}

func (fakeUserLoginService) UserLogin(attempt *playersession.Attempt, _ string, _ int32, _ message.LoginType) error {
	if _, ok := attempt.Commit(models.User{AccountId: "test", PlayerId: 42}, &playersession.Slot{}); !ok {
		return playersession.ErrStaleAttempt
	}
	attempt.Reply(&message.S2C_Login{LoginResult: 1})
	return nil
}

func TestSubmitLoginReturnsBeforeAuthenticatorFinishes(t *testing.T) {
	authenticator := &blockingAuthenticator{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	attempts := &fakeLoginAttempts{state: playersession.NewState(), heartbeats: make(chan gate.ConnectionID, 1)}
	system := actor.NewActorSystem(time.Second)
	scope, err := system.NewScope("login-manager-test")
	if err != nil {
		t.Fatalf("创建 Scope: %v", err)
	}
	manager, err := NewLoginManager(context.Background(), scope, fakeUserLoginService{}, LoginManagerOptions{
		Authenticator: authenticator,
		Workers:       1,
		QueueSize:     1,
		Timeout:       time.Second,
		Attempts:      attempts,
	})
	if err != nil {
		t.Fatalf("创建 LoginManager: %v", err)
	}
	defer func() {
		manager.Close()
		if err := system.Stop(context.Background()); err != nil {
			t.Errorf("停止 ActorSystem: %v", err)
		}
	}()

	agent := &asyncLoginAgent{
		testConnectionAgent: testConnectionAgent{id: 7},
		writes:              make(chan loginWrite, 1),
	}
	attempts.state.Register(agent)
	startedAt := time.Now()
	if err := manager.SubmitLogin(&message.C2S_Login{ServerId: 1}, agent, 42); err != nil {
		t.Fatalf("提交登录任务: %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed > 100*time.Millisecond {
		t.Fatalf("提交登录任务耗时 %v，阻塞了消息处理线程", elapsed)
	}
	select {
	case <-authenticator.started:
	case <-time.After(time.Second):
		t.Fatal("认证 Worker 未开始执行")
	}
	select {
	case <-agent.writes:
		t.Fatal("认证尚未完成就写回了登录响应")
	case <-time.After(20 * time.Millisecond):
	}

	close(authenticator.release)
	select {
	case write := <-agent.writes:
		if write.sequence != 42 {
			t.Fatalf("登录响应序号 = %d，期望 42", write.sequence)
		}
		response, ok := write.message.(*message.S2C_Login)
		if !ok || response.LoginResult != 1 {
			t.Fatalf("登录响应 = %#v，期望成功响应", write.message)
		}
	case <-time.After(time.Second):
		t.Fatal("认证完成后未收到登录响应")
	}
	select {
	case <-attempts.heartbeats:
	case <-time.After(time.Second):
		t.Fatal("登录成功后未刷新心跳")
	}
}
