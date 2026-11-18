package provision

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeMaterializer struct {
	failures int
	calls    []string
}

type failureRecorder struct {
	intents chan Intent
}

func (r failureRecorder) HandleProvisionFailure(_ context.Context, intent Intent) error {
	r.intents <- intent
	return nil
}

func (m *fakeMaterializer) Apply(ctx context.Context, intent Intent) error {
	m.calls = append(m.calls, intent.AccountID)
	if m.failures > 0 {
		m.failures--
		return errors.New("materializer unavailable")
	}
	return nil
}

type blockingMaterializer struct {
	started chan struct{}
}

type shutdownMaterializer struct {
	started chan context.Context
	release chan struct{}
}

func (m shutdownMaterializer) Apply(ctx context.Context, _ Intent) error {
	m.started <- ctx
	<-m.release
	return ctx.Err()
}

func TestWorkerStopTimeoutCancelsScanAndCanWaitAgain(t *testing.T) {
	repository := NewMemoryRepository()
	if err := repository.Save(context.Background(), &Intent{AccountID: "shutdown", Status: StatusPending}); err != nil {
		t.Fatal(err)
	}
	materializer := shutdownMaterializer{started: make(chan context.Context, 1), release: make(chan struct{})}
	worker, err := NewWorker(repository, materializer, failureRecorder{intents: make(chan Intent, 1)})
	if err != nil {
		t.Fatal(err)
	}
	worker.interval = time.Millisecond
	worker.Start()
	done := worker.done
	release := sync.OnceFunc(func() { close(materializer.release) })
	t.Cleanup(func() { release(); <-done })
	var scanContext context.Context
	select {
	case scanContext = <-materializer.started:
	case <-time.After(time.Second):
		t.Fatal("scan did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := worker.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first Stop = %v", err)
	}
	if !errors.Is(scanContext.Err(), context.Canceled) {
		t.Fatalf("scan was not canceled after Stop timed out: %v", scanContext.Err())
	}
	retryContext, cancelRetry := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelRetry()
	if err := worker.Stop(retryContext); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop reported completion before the scan exited: %v", err)
	}
	release()
	if err := worker.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	worker.Start()
	if worker.done != done {
		t.Fatal("stopped Worker started another execution loop")
	}
	intent, err := repository.FindByAccount(context.Background(), "shutdown")
	if err != nil || intent == nil || intent.Status != StatusPending || intent.Attempts != 0 {
		t.Fatalf("interrupted intent was not kept for retry: %#v, %v", intent, err)
	}
}

func (m blockingMaterializer) Apply(ctx context.Context, _ Intent) error {
	close(m.started)
	<-ctx.Done()
	return ctx.Err()
}

func TestWorkerCancellationReachesMaterializerAndKeepsIntentForRetry(t *testing.T) {
	repository := NewMemoryRepository()
	if err := repository.Save(context.Background(), &Intent{
		AccountID: "cancelled-account",
		IntentID:  "cancelled-intent",
		Version:   CurrentVersion,
		Status:    StatusPending,
	}); err != nil {
		t.Fatal(err)
	}
	materializer := blockingMaterializer{started: make(chan struct{})}
	worker, err := NewWorker(repository, materializer, failureRecorder{intents: make(chan Intent, 1)})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	completed := make(chan error, 1)
	go func() { completed <- worker.RunOnce(ctx) }()
	select {
	case <-materializer.started:
	case <-time.After(time.Second):
		t.Fatal("materialization did not start")
	}
	cancel()
	select {
	case err := <-completed:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled RunOnce = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("materializer did not observe RunOnce cancellation")
	}
	intent, err := repository.FindByAccount(context.Background(), "cancelled-account")
	if err != nil || intent == nil || intent.Status != StatusPending {
		t.Fatalf("cancelled intent = %#v, %v", intent, err)
	}
	worker.materializer = &fakeMaterializer{}
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("retry RunOnce = %v", err)
	}
	intent, err = repository.FindByAccount(context.Background(), "cancelled-account")
	if err != nil || intent == nil || intent.Status != StatusApplied {
		t.Fatalf("retried intent = %#v, %v", intent, err)
	}
}

func TestWorkerMarksIntentAppliedAfterIdempotentMaterialization(t *testing.T) {
	repository := NewMemoryRepository()
	now := time.Now()
	if err := repository.Save(context.Background(), &Intent{
		AccountID:     "account-1",
		IntentID:      "intent-1",
		Version:       CurrentVersion,
		Status:        StatusPending,
		NextAttemptAt: now.Add(-time.Second),
	}); err != nil {
		t.Fatalf("保存 Intent: %v", err)
	}
	materializer := &fakeMaterializer{}
	failure := failureRecorder{intents: make(chan Intent, 1)}
	worker, err := NewWorker(repository, materializer, failure)
	if err != nil {
		t.Fatalf("创建 Worker: %v", err)
	}
	worker.now = func() time.Time { return now }

	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("处理 Intent: %v", err)
	}
	intent, err := repository.FindByAccount(context.Background(), "account-1")
	if err != nil {
		t.Fatalf("读取 Intent: %v", err)
	}
	if intent == nil || intent.Status != StatusApplied || len(materializer.calls) != 1 {
		t.Fatalf("处理结果 = %#v，调用次数 = %d", intent, len(materializer.calls))
	}
}

func TestWorkerRetriesAndEventuallyMarksIntentFailed(t *testing.T) {
	repository := NewMemoryRepository()
	now := time.Now()
	if err := repository.Save(context.Background(), &Intent{
		AccountID:     "account-2",
		IntentID:      "intent-2",
		Version:       CurrentVersion,
		Status:        StatusPending,
		NextAttemptAt: now.Add(-time.Second),
	}); err != nil {
		t.Fatalf("保存 Intent: %v", err)
	}
	materializer := &fakeMaterializer{failures: 2}
	recorder := failureRecorder{intents: make(chan Intent, 1)}
	worker, err := NewWorker(repository, materializer, recorder)
	if err != nil {
		t.Fatalf("创建 Worker: %v", err)
	}
	worker.now = func() time.Time { return now }
	worker.maxAttempts = 2

	if err := worker.RunOnce(context.Background()); err == nil {
		t.Fatal("第一次失败未返回错误")
	}
	intent, err := repository.FindByAccount(context.Background(), "account-2")
	if err != nil {
		t.Fatalf("读取第一次结果: %v", err)
	}
	if intent.Status != StatusPending || intent.Attempts != 1 {
		t.Fatalf("第一次重试状态 = %#v", intent)
	}

	intent.NextAttemptAt = now.Add(-time.Second)
	if err := repository.Save(context.Background(), intent); err != nil {
		t.Fatalf("准备第二次重试: %v", err)
	}
	if err := worker.RunOnce(context.Background()); err == nil {
		t.Fatal("第二次失败未返回错误")
	}
	intent, err = repository.FindByAccount(context.Background(), "account-2")
	if err != nil {
		t.Fatalf("读取最终结果: %v", err)
	}
	if intent.Status != StatusFailed || intent.Attempts != 2 {
		t.Fatalf("最终失败状态 = %#v", intent)
	}
	select {
	case failed := <-recorder.intents:
		if failed.AccountID != "account-2" || failed.Status != StatusFailed {
			t.Fatalf("终态失败回调 = %#v", failed)
		}
	case <-time.After(time.Second):
		t.Fatal("终态失败未触发会话终止回调")
	}
}
