package internal

import (
	"context"
	"gameserver/common"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	"gameserver/common/schedule"
	"gameserver/modules/room/internal/managers"
	"gameserver/modules/room/matchentry"
	"gameserver/modules/room/participant"
	"slices"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
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

func (j *fakeRoomJob) Stop(context.Context) error {
	j.stopped = true
	*j.events = append(*j.events, "job.stop")
	return nil
}

type fakeRoomMaintenance struct {
	runs   int
	events *[]string
}

func (m *fakeRoomMaintenance) Maintain() {
	m.runs++
}

func (m *fakeRoomMaintenance) Stop(context.Context) error {
	*m.events = append(*m.events, "maintenance.stop")
	return nil
}

func (m *fakeRoomMaintenance) HandleRecordOperate(int64, string, string) *message.S2C_RecordGameOperate {
	return &message.S2C_RecordGameOperate{}
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

type expiredBudgetRoomJob struct{}

func (expiredBudgetRoomJob) Stop(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

type roomCleanupTestProjection struct{ roomID string }

func (p *roomCleanupTestProjection) Apply(ctx context.Context, desired []participant.DesiredTeamRoom) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, entry := range desired {
		p.roomID = entry.RoomID
	}
	return nil
}

type roomCleanupTestMessenger struct{}

func (roomCleanupTestMessenger) Send([]int64, proto.Message)              {}
func (roomCleanupTestMessenger) SendExcept([]int64, int64, proto.Message) {}

type roomCleanupPersistenceActor struct {
	actor.BaseActor
	projection  *roomCleanupTestProjection
	savedRoomID string
}

func (a *roomCleanupPersistenceActor) OnStop(context.Context) error {
	a.savedRoomID = a.projection.roomID
	return nil
}

func TestRoomModuleCleansTeamProjectionAfterShutdownBudgetExpires(t *testing.T) {
	system := actor.NewActorSystem(time.Second)
	t.Cleanup(func() {
		if err := system.Stop(context.Background()); err != nil {
			t.Error(err)
		}
	})
	roomScope, _ := system.NewScope("room-shutdown")
	gameScope, _ := system.NewScope("game-shutdown")
	projection := &roomCleanupTestProjection{}
	definition, err := actor.Define(gameScope, func(context.Context, int64) (*roomCleanupPersistenceActor, error) {
		return &roomCleanupPersistenceActor{projection: projection}, nil
	}, actor.DefinitionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	team, err := definition.GetOrCreate(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	maintenance, err := managers.NewRoomManager(roomScope, projection, roomCleanupTestMessenger{}, time.Now, func() string { return "room-1" })
	if err != nil {
		t.Fatal(err)
	}
	result := maintenance.AcceptMatch(context.Background(), matchentry.Admission{
		MatchID: "match-1", Teams: []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
	})
	if result.Status != matchentry.Accepted || projection.roomID != "room-1" {
		t.Fatal("Room setup failed")
	}
	roomModule := NewModule(roomScope, nil, maintenance)
	roomModule.job = expiredBudgetRoomJob{}
	roomModule.OnDestroy()
	if err := gameScope.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if team.savedRoomID != "" {
		t.Fatalf("game shutdown persisted stale RoomID %q after Room cleanup exhausted its budget", team.savedRoomID)
	}
}
