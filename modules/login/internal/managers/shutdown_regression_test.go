package managers

import (
	"context"
	"errors"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/modules/login/internal/models"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type shutdownAuthenticator struct {
	started chan context.Context
	release chan struct{}
	calls   atomic.Int32
}

func (a *shutdownAuthenticator) Authenticate(ctx context.Context, _ gate.Agent, _ *message.C2S_Login) *models.LoginResponse {
	if a.calls.Add(1) == 1 {
		a.started <- ctx
		// Keep the first call alive until the test checks cancellation and retries Close.
		<-a.release
	}
	return &models.LoginResponse{Openid: "open-id"}
}

func TestAuthWorkerPoolCloseTimeoutCancelsWorkAndSkipsQueuedAuthentication(t *testing.T) {
	authenticator := &shutdownAuthenticator{started: make(chan context.Context, 1), release: make(chan struct{})}
	pool, err := newAuthWorkerPool(authenticator, 1, 1, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	release := sync.OnceFunc(func() { close(authenticator.release) })
	t.Cleanup(func() { release(); _ = pool.close(context.Background()) })
	responses := make(chan *models.LoginResponse, 2)
	task := authTask{complete: func(response *models.LoginResponse) { responses <- response }}
	if err := pool.submit(task); err != nil {
		t.Fatal(err)
	}
	var authenticationContext context.Context
	select {
	case authenticationContext = <-authenticator.started:
	case <-time.After(time.Second):
		t.Fatal("authentication did not start")
	}
	if err := pool.submit(task); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := pool.close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close = %v", err)
	}
	if !errors.Is(authenticationContext.Err(), context.Canceled) {
		t.Errorf("in-flight authentication was not canceled: %v", authenticationContext.Err())
	}
	select {
	case <-pool.done:
		t.Fatal("pool reported completion before in-flight authentication returned")
	default:
	}
	release()
	if err := pool.close(context.Background()); err != nil {
		t.Fatalf("final Close = %v", err)
	}
	if authenticator.calls.Load() != 1 {
		t.Errorf("authenticator calls = %d; queued authentication started after shutdown timeout", authenticator.calls.Load())
	}
	for range 2 {
		select {
		case response := <-responses:
			if response != nil && response.ErrCode == 0 {
				t.Errorf("canceled authentication completed successfully: %#v", response)
			}
		default:
			t.Error("canceled task did not receive a failure completion")
		}
	}
}
