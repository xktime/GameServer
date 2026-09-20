package provision

import (
	"context"
	"errors"
	model "gameserver/modules/game/internal/provision"
	"testing"
	"time"
)

type fakeMaterializer struct {
	failures int
	calls    []string
}

type failureRecorder struct {
	intents chan model.Intent
}

func (r failureRecorder) HandleProvisionFailure(_ context.Context, intent model.Intent) error {
	r.intents <- intent
	return nil
}

func (m *fakeMaterializer) Apply(intent model.Intent) error {
	m.calls = append(m.calls, intent.AccountID)
	if m.failures > 0 {
		m.failures--
		return errors.New("materializer unavailable")
	}
	return nil
}

func TestWorkerMarksIntentAppliedAfterIdempotentMaterialization(t *testing.T) {
	repository := NewMemoryRepository()
	now := time.Now()
	if err := repository.Save(&model.Intent{
		AccountID:     "account-1",
		IntentID:      "intent-1",
		Version:       model.CurrentVersion,
		Status:        model.StatusPending,
		NextAttemptAt: now.Add(-time.Second),
	}); err != nil {
		t.Fatalf("保存 Intent: %v", err)
	}
	materializer := &fakeMaterializer{}
	failure := failureRecorder{intents: make(chan model.Intent, 1)}
	worker, err := NewWorker(repository, materializer, failure)
	if err != nil {
		t.Fatalf("创建 Worker: %v", err)
	}
	worker.now = func() time.Time { return now }

	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("处理 Intent: %v", err)
	}
	intent, err := repository.FindByAccount("account-1")
	if err != nil {
		t.Fatalf("读取 Intent: %v", err)
	}
	if intent == nil || intent.Status != model.StatusApplied || len(materializer.calls) != 1 {
		t.Fatalf("处理结果 = %#v，调用次数 = %d", intent, len(materializer.calls))
	}
}

func TestWorkerRetriesAndEventuallyMarksIntentFailed(t *testing.T) {
	repository := NewMemoryRepository()
	now := time.Now()
	if err := repository.Save(&model.Intent{
		AccountID:     "account-2",
		IntentID:      "intent-2",
		Version:       model.CurrentVersion,
		Status:        model.StatusPending,
		NextAttemptAt: now.Add(-time.Second),
	}); err != nil {
		t.Fatalf("保存 Intent: %v", err)
	}
	materializer := &fakeMaterializer{failures: 2}
	recorder := failureRecorder{intents: make(chan model.Intent, 1)}
	worker, err := NewWorker(repository, materializer, recorder)
	if err != nil {
		t.Fatalf("创建 Worker: %v", err)
	}
	worker.now = func() time.Time { return now }
	worker.maxAttempts = 2

	if err := worker.RunOnce(context.Background()); err == nil {
		t.Fatal("第一次失败未返回错误")
	}
	intent, err := repository.FindByAccount("account-2")
	if err != nil {
		t.Fatalf("读取第一次结果: %v", err)
	}
	if intent.Status != model.StatusPending || intent.Attempts != 1 {
		t.Fatalf("第一次重试状态 = %#v", intent)
	}

	intent.NextAttemptAt = now.Add(-time.Second)
	if err := repository.Save(intent); err != nil {
		t.Fatalf("准备第二次重试: %v", err)
	}
	if err := worker.RunOnce(context.Background()); err == nil {
		t.Fatal("第二次失败未返回错误")
	}
	intent, err = repository.FindByAccount("account-2")
	if err != nil {
		t.Fatalf("读取最终结果: %v", err)
	}
	if intent.Status != model.StatusFailed || intent.Attempts != 2 {
		t.Fatalf("最终失败状态 = %#v", intent)
	}
	select {
	case failed := <-recorder.intents:
		if failed.AccountID != "account-2" || failed.Status != model.StatusFailed {
			t.Fatalf("终态失败回调 = %#v", failed)
		}
	case <-time.After(time.Second):
		t.Fatal("终态失败未触发会话终止回调")
	}
}
