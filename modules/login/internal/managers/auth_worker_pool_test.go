package managers

import (
	"context"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/modules/login/internal/models"
	"sync"
	"testing"
	"time"
)

type blockingAuthenticator struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (a *blockingAuthenticator) Authenticate(ctx context.Context, _ gate.Agent, _ *message.C2S_Login) *models.LoginResponse {
	a.once.Do(func() { close(a.started) })
	select {
	case <-a.release:
		return &models.LoginResponse{ErrCode: 0, Openid: "open-id"}
	case <-ctx.Done():
		return &models.LoginResponse{ErrCode: -1, ErrMsg: ctx.Err().Error()}
	}
}

func TestAuthWorkerPoolRejectsTasksAfterQueueIsFull(t *testing.T) {
	authenticator := &blockingAuthenticator{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	p, err := newAuthWorkerPool(authenticator, 1, 1, time.Second)
	if err != nil {
		t.Fatalf("创建认证 Worker Pool: %v", err)
	}
	defer p.close()

	completed := make(chan struct{}, 2)
	task := func() authTask {
		return authTask{
			agent:    &testConnectionAgent{id: 1},
			request:  &message.C2S_Login{},
			complete: func(*models.LoginResponse) { completed <- struct{}{} },
		}
	}
	if err := p.submit(task()); err != nil {
		t.Fatalf("提交首个认证任务: %v", err)
	}
	select {
	case <-authenticator.started:
	case <-time.After(time.Second):
		t.Fatal("认证 Worker 未开始执行")
	}
	if err := p.submit(task()); err != nil {
		t.Fatalf("提交队列任务: %v", err)
	}
	if err := p.submit(task()); err != ErrAuthQueueFull {
		t.Fatalf("队列已满时错误 = %v，期望 ErrAuthQueueFull", err)
	}

	close(authenticator.release)
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("认证任务未完成")
	}
}

func TestAuthWorkerPoolPassesTimeoutToAuthenticator(t *testing.T) {
	authenticator := &blockingAuthenticator{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	p, err := newAuthWorkerPool(authenticator, 1, 1, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("创建认证 Worker Pool: %v", err)
	}
	defer p.close()

	responses := make(chan *models.LoginResponse, 1)
	if err := p.submit(authTask{
		agent:    &testConnectionAgent{id: 1},
		request:  &message.C2S_Login{},
		complete: func(response *models.LoginResponse) { responses <- response },
	}); err != nil {
		t.Fatalf("提交认证任务: %v", err)
	}
	select {
	case response := <-responses:
		if response.ErrCode == 0 {
			t.Fatal("超时认证任务不应成功")
		}
	case <-time.After(time.Second):
		t.Fatal("认证 Worker 未按超时返回")
	}
}

var _ gate.ConnectionAgent = (*testConnectionAgent)(nil)
