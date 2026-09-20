package managers

import (
	"context"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"sync"
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
	mu         sync.Mutex
	next       uint64
	current    LoginAttempt
	heartbeats chan gate.ConnectionID
}

func (a *fakeLoginAttempts) BeginLogin(connectionID gate.ConnectionID) (LoginAttempt, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.next++
	a.current = LoginAttempt{ConnectionID: connectionID, Sequence: a.next}
	return a.current, nil
}

func (a *fakeLoginAttempts) WithCurrentLogin(attempt LoginAttempt, fn func()) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.current != attempt || fn == nil {
		return false
	}
	fn()
	return true
}

func (a *fakeLoginAttempts) UpdateHeartbeat(connectionID gate.ConnectionID) {
	a.heartbeats <- connectionID
}

type fakeUserLoginService struct{}

func (fakeUserLoginService) UserLogin(gate.Agent, string, int32, message.LoginType) *message.S2C_Login {
	return &message.S2C_Login{LoginResult: 1}
}

func TestSubmitLoginReturnsBeforeAuthenticatorFinishes(t *testing.T) {
	authenticator := &blockingAuthenticator{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	attempts := &fakeLoginAttempts{heartbeats: make(chan gate.ConnectionID, 1)}
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
