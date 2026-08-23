package schedule

import (
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

	job.Stop()
	stoppedAt := calls.Load()
	time.Sleep(10 * time.Millisecond)
	if got := calls.Load(); got != stoppedAt {
		t.Fatalf("Stop 后仍执行任务：停止时 %d 次，当前 %d 次", stoppedAt, got)
	}
	job.Stop()
}
