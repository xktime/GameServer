package internal

import (
	"gameserver/common"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	"gameserver/common/schedule"
	"slices"
	"testing"
	"time"
)

type fakeRoomScheduler struct {
	interval time.Duration
	callback func()
	job      *fakeRoomJob
	events   *[]string
}

func (s *fakeRoomScheduler) Every(interval time.Duration, callback func()) schedule.Job {
	s.interval = interval
	s.callback = callback
	s.job = &fakeRoomJob{events: s.events}
	return s.job
}

func (s *fakeRoomScheduler) Tick() {
	if s.job != nil && !s.job.stopped {
		s.callback()
	}
}

type fakeRoomJob struct {
	stopped bool
	events  *[]string
}

func (j *fakeRoomJob) Stop() {
	j.stopped = true
	*j.events = append(*j.events, "job.stop")
}

type fakeRoomMaintenance struct {
	runs   int
	events *[]string
}

func (m *fakeRoomMaintenance) Maintain() {
	m.runs++
}

func (m *fakeRoomMaintenance) Stop() {
	*m.events = append(*m.events, "maintenance.stop")
}

func (m *fakeRoomMaintenance) HandleRecordOperate(int64, string, string) *message.S2C_RecordGameOperate {
	return &message.S2C_RecordGameOperate{}
}

func (m *fakeRoomMaintenance) PlayerOffline(int64) bool {
	return false
}

func TestModuleOwnsTenSecondRoomMaintenance(t *testing.T) {
	previousSkeleton := skeleton
	skeleton = common.NewSkeleton()
	t.Cleanup(func() { skeleton = previousSkeleton })
	events := make([]string, 0, 2)
	system := actor.NewActorSystem(time.Second)
	scope, err := system.NewScope("room-test")
	if err != nil {
		t.Fatalf("NewScope: %v", err)
	}
	scheduler := &fakeRoomScheduler{events: &events}
	maintenance := &fakeRoomMaintenance{events: &events}
	roomModule := NewModule(scope, scheduler, maintenance)

	roomModule.OnInit()
	if scheduler.interval != 10*time.Second {
		t.Fatalf("Room 维护周期 = %v，期望 10s", scheduler.interval)
	}
	scheduler.Tick()
	if maintenance.runs != 1 {
		t.Fatalf("调度后 Room 维护次数 = %d，期望 1", maintenance.runs)
	}

	roomModule.OnDestroy()
	scheduler.Tick()
	if maintenance.runs != 1 || !scheduler.job.stopped {
		t.Fatalf("模块关闭后的调度状态 = runs:%d stopped:%t", maintenance.runs, scheduler.job.stopped)
	}
	if !slices.Equal(events, []string{"job.stop", "maintenance.stop"}) {
		t.Fatalf("模块关闭顺序 = %#v", events)
	}
}
