package internal

import (
	"gameserver/common"
	"gameserver/common/base/actor"
	"gameserver/common/schedule"
	"testing"
	"time"
)

type fakeMatchScheduler struct {
	interval time.Duration
	callback func()
	job      *fakeMatchJob
}

func (s *fakeMatchScheduler) Every(interval time.Duration, callback func()) schedule.Job {
	s.interval = interval
	s.callback = callback
	s.job = &fakeMatchJob{}
	return s.job
}

func (s *fakeMatchScheduler) Tick() {
	if s.job != nil && !s.job.stopped {
		s.callback()
	}
}

type fakeMatchJob struct {
	stopped bool
}

func (j *fakeMatchJob) Stop() {
	j.stopped = true
}

type fakeMatchCycle struct {
	ticks int
}

func (c *fakeMatchCycle) OnTimer() {
	c.ticks++
}

func TestModuleOwnsTenSecondMatchCycle(t *testing.T) {
	previousSkeleton := skeleton
	skeleton = common.NewSkeleton()
	t.Cleanup(func() { skeleton = previousSkeleton })
	actor.Init(1000)
	scheduler := &fakeMatchScheduler{}
	cycle := &fakeMatchCycle{}
	matchModule := NewModule(scheduler, cycle)

	matchModule.OnInit()
	if scheduler.interval != 10*time.Second {
		t.Fatalf("匹配周期 = %v，期望 10s", scheduler.interval)
	}
	scheduler.Tick()
	if cycle.ticks != 1 {
		t.Fatalf("调度后匹配 Tick 次数 = %d，期望 1", cycle.ticks)
	}

	matchModule.OnDestroy()
	scheduler.Tick()
	if cycle.ticks != 1 || !scheduler.job.stopped {
		t.Fatalf("模块关闭后的调度状态 = ticks:%d stopped:%t", cycle.ticks, scheduler.job.stopped)
	}
}
