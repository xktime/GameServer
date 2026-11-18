package schedule

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSchedulerRunsJobSeriallyAndStops(t *testing.T) {
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})
	var calls atomic.Int32

	job := NewScheduler().Every(time.Millisecond, func() {
		switch calls.Add(1) {
		case 1:
			close(firstStarted)
			<-releaseFirst
		case 2:
			close(secondStarted)
		}
	})

	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("周期任务未启动")
	}
	select {
	case <-secondStarted:
		t.Fatal("上一次执行未完成时启动了重叠任务")
	case <-time.After(20 * time.Millisecond):
	}

	close(releaseFirst)
	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("上一次执行完成后没有继续调度")
	}

	if err := job.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	stoppedAt := calls.Load()
	time.Sleep(10 * time.Millisecond)
	if got := calls.Load(); got != stoppedAt {
		t.Fatalf("Stop 后仍执行任务：停止时 %d 次，当前 %d 次", stoppedAt, got)
	}
	if err := job.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestStopDeadlineDoesNotWaitForeverForCallback(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	var calls atomic.Int32
	job := NewScheduler().Every(time.Millisecond, func() {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
	})
	t.Cleanup(func() { releaseOnce(); _ = job.Stop(context.Background()) })
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := job.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop with running callback = %v", err)
	}
	releaseOnce()
	if err := job.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("callback ran again after Stop: %d calls", got)
	}
}
