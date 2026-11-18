package managers

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	"gameserver/modules/room/matchentry"
	"gameserver/modules/room/participant"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

type fakeTeamRoomProjection struct {
	applied [][]participant.DesiredTeamRoom
	err     error
}

type teamRoomProjectionFunc func(context.Context, []participant.DesiredTeamRoom) error

func (f teamRoomProjectionFunc) Apply(ctx context.Context, desired []participant.DesiredTeamRoom) error {
	return f(ctx, desired)
}

type reorderedTeamRoomProjection struct {
	joinStarted chan struct{}
	releaseJoin chan struct{}
	joinOnce    sync.Once
	mu          sync.Mutex
	applied     [][]participant.DesiredTeamRoom
}

func (p *reorderedTeamRoomProjection) Apply(ctx context.Context, desired []participant.DesiredTeamRoom) error {
	isFirstJoin := false
	if len(desired) == 1 && desired[0].RoomID != "" {
		p.joinOnce.Do(func() {
			isFirstJoin = true
			close(p.joinStarted)
		})
	}
	if isFirstJoin {
		<-p.releaseJoin
	}
	p.mu.Lock()
	p.applied = append(p.applied, append([]participant.DesiredTeamRoom(nil), desired...))
	p.mu.Unlock()
	return nil
}

func (p *reorderedTeamRoomProjection) history() [][]participant.DesiredTeamRoom {
	p.mu.Lock()
	defer p.mu.Unlock()
	history := make([][]participant.DesiredTeamRoom, len(p.applied))
	for index := range p.applied {
		history[index] = append([]participant.DesiredTeamRoom(nil), p.applied[index]...)
	}
	return history
}

func (f *fakeTeamRoomProjection) Apply(ctx context.Context, desired []participant.DesiredTeamRoom) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.applied = append(f.applied, append([]participant.DesiredTeamRoom(nil), desired...))
	return f.err
}

type deliveredRoomMessage struct {
	playerIDs []int64
	excluded  int64
	msg       proto.Message
}

type fakePlayerMessenger struct {
	delivered []deliveredRoomMessage
}

type unrelatedRoomTestActor struct {
	actor.BaseActor
}

func (f *fakePlayerMessenger) Send(playerIDs []int64, msg proto.Message) {
	f.delivered = append(f.delivered, deliveredRoomMessage{
		playerIDs: append([]int64(nil), playerIDs...),
		msg:       msg,
	})
}

func (f *fakePlayerMessenger) SendExcept(playerIDs []int64, excludedPlayerID int64, msg proto.Message) {
	f.delivered = append(f.delivered, deliveredRoomMessage{
		playerIDs: append([]int64(nil), playerIDs...),
		excluded:  excludedPlayerID,
		msg:       msg,
	})
}

func newTestRoomManager(
	t *testing.T,
	projection participant.TeamRoomProjection,
	messenger participant.PlayerMessenger,
	now func() time.Time,
	newRoomID func() string,
) *RoomManager {
	t.Helper()
	system := actor.NewActorSystem(time.Second)
	scope, err := system.NewScope("room-test")
	if err != nil {
		t.Fatalf("NewScope: %v", err)
	}
	t.Cleanup(func() {
		if err := system.Stop(context.Background()); err != nil {
			t.Errorf("Stop ActorSystem: %v", err)
		}
	})
	manager, err := NewRoomManager(scope, projection, messenger, now, newRoomID)
	if err != nil {
		t.Fatalf("NewRoomManager: %v", err)
	}
	return manager
}

func TestCanceledMatchCreationCleansActorAndAllowsRetry(t *testing.T) {
	projection := &fakeTeamRoomProjection{}
	messenger := &fakePlayerMessenger{}
	nextRoomID := 0
	manager := newTestRoomManager(t, projection, messenger, time.Now, func() string {
		nextRoomID++
		return "room-" + strconv.Itoa(nextRoomID)
	})
	admission := matchentry.Admission{
		MatchID: "match-1",
		Teams:   []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result := manager.AcceptMatch(ctx, admission); result.Status != matchentry.Retryable {
		t.Fatalf("canceled admission = %#v; want Retryable", result)
	}
	lookupContext, cancelLookup := context.WithTimeout(context.Background(), time.Second)
	defer cancelLookup()
	if _, err := manager.roomDefinition.Lookup(lookupContext, "room-1"); !errors.Is(err, actor.ErrActorNotFound) {
		t.Fatalf("canceled admission left an Actor registered: %v", err)
	}
	if len(manager.rooms) != 0 || len(manager.matchRooms) != 0 || len(manager.playerRooms) != 0 || len(manager.teamRooms) != 0 {
		t.Fatal("canceled admission left Room indexes populated")
	}
	if len(projection.applied) != 0 || len(messenger.delivered) != 0 {
		t.Fatal("canceled admission projected or notified an uncommitted Room")
	}
	if result := manager.AcceptMatch(context.Background(), admission); result.Status != matchentry.Accepted || result.RoomID != "room-2" {
		t.Fatalf("retry admission = %#v; want room-2 accepted", result)
	}
}

func TestAcceptMatchCommitsRoomAndNotifiesOnlyRealPlayers(t *testing.T) {
	projection := &fakeTeamRoomProjection{}
	messenger := &fakePlayerMessenger{}
	manager := newTestRoomManager(t,
		projection,
		messenger,
		func() time.Time { return time.Unix(100, 0) },
		func() string { return "room-1" },
	)

	result := manager.AcceptMatch(context.Background(), matchentry.Admission{
		MatchID: "match-1",
		Teams: []matchentry.MatchedTeam{
			{TeamID: 7, PlayerIDs: []int64{42, 43}},
			{TeamID: -1, PlayerIDs: []int64{-2}, IsRobot: true},
		},
	})

	if result.Status != matchentry.Accepted || result.RoomID != "room-1" {
		t.Fatalf("AcceptMatch() = %#v", result)
	}
	wantProjection := []participant.DesiredTeamRoom{{TeamID: 7, RoomID: "room-1"}}
	if len(projection.applied) != 1 || !slices.Equal(projection.applied[0], wantProjection) {
		t.Fatalf("Team Room projection = %#v", projection.applied)
	}
	if len(messenger.delivered) != 1 || !slices.Equal(messenger.delivered[0].playerIDs, []int64{42, 43}) {
		t.Fatalf("MatchResult recipients = %#v", messenger.delivered)
	}
	matchResult, ok := messenger.delivered[0].msg.(*message.S2C_MatchResult)
	if !ok || matchResult.RoomId != "room-1" || len(matchResult.PlayerInfos) != 3 {
		t.Fatalf("MatchResult = %#v", messenger.delivered[0].msg)
	}
	if matchResult.PlayerInfos[2].PlayerId != "-2" || !matchResult.PlayerInfos[2].IsRobot {
		t.Fatalf("synthetic Robot info = %#v", matchResult.PlayerInfos[2])
	}
}

func TestAcceptMatchNotifiesAfterCallerCancellationFollowingCommit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	projection := &fakeTeamRoomProjection{}
	messenger := &fakePlayerMessenger{}
	manager := newTestRoomManager(t, teamRoomProjectionFunc(func(ctx context.Context, desired []participant.DesiredTeamRoom) error {
		cancel()
		return projection.Apply(ctx, desired)
	}), messenger, time.Now, func() string { return "room-1" })
	admission := matchentry.Admission{
		MatchID: "match-1",
		Teams:   []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
	}

	result := manager.AcceptMatch(ctx, admission)
	if result.Status != matchentry.Accepted || result.RoomID != "room-1" {
		t.Fatalf("AcceptMatch() = %#v", result)
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("caller context was not canceled after Room commit")
	}
	if retry := manager.AcceptMatch(context.Background(), admission); retry.Status != matchentry.AlreadyAccepted || retry.RoomID != result.RoomID {
		t.Fatalf("duplicate admission = %#v", retry)
	}
	if len(messenger.delivered) != 1 || !slices.Equal(messenger.delivered[0].playerIDs, []int64{42}) {
		t.Fatalf("MatchResult after caller cancellation = %#v", messenger.delivered)
	}
	matchResult, ok := messenger.delivered[0].msg.(*message.S2C_MatchResult)
	if !ok || matchResult.RoomId != result.RoomID {
		t.Fatalf("MatchResult = %#v", messenger.delivered[0].msg)
	}
	wantProjection := []participant.DesiredTeamRoom{{TeamID: 7, RoomID: "room-1"}}
	if len(projection.applied) != 1 || !slices.Equal(projection.applied[0], wantProjection) {
		t.Fatalf("Team Room projection after caller cancellation = %#v", projection.applied)
	}
}

func TestAcceptMatchReturnsExistingRoomForDuplicateMatchID(t *testing.T) {
	projection := &fakeTeamRoomProjection{}
	messenger := &fakePlayerMessenger{}
	roomIDs := []string{"room-1", "room-2"}
	manager := newTestRoomManager(t,
		projection,
		messenger,
		func() time.Time { return time.Unix(100, 0) },
		func() string {
			roomID := roomIDs[0]
			roomIDs = roomIDs[1:]
			return roomID
		},
	)
	admission := matchentry.Admission{
		MatchID: "match-1",
		Teams:   []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
	}

	first := manager.AcceptMatch(context.Background(), admission)
	second := manager.AcceptMatch(context.Background(), admission)

	if first.Status != matchentry.Accepted || second.Status != matchentry.AlreadyAccepted || second.RoomID != first.RoomID {
		t.Fatalf("duplicate results = %#v, %#v", first, second)
	}
	if len(projection.applied) != 1 || len(messenger.delivered) != 1 {
		t.Fatalf("duplicate admission repeated side effects: projection=%d messages=%d", len(projection.applied), len(messenger.delivered))
	}
}

func TestAcceptMatchRejectsParticipantAlreadyInActiveRoom(t *testing.T) {
	projection := &fakeTeamRoomProjection{}
	messenger := &fakePlayerMessenger{}
	roomNumber := 0
	manager := newTestRoomManager(t,
		projection,
		messenger,
		func() time.Time { return time.Unix(100, 0) },
		func() string {
			roomNumber++
			return "room-" + strconv.Itoa(roomNumber)
		},
	)
	manager.AcceptMatch(context.Background(), matchentry.Admission{
		MatchID: "match-1",
		Teams:   []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
	})

	result := manager.AcceptMatch(context.Background(), matchentry.Admission{
		MatchID: "match-2",
		Teams:   []matchentry.MatchedTeam{{TeamID: 8, PlayerIDs: []int64{42, 43}}},
	})

	if result.Status != matchentry.Rejected || !slices.Equal(result.RejectedTeamIDs, []int64{8}) {
		t.Fatalf("conflicting admission = %#v", result)
	}
	if len(projection.applied) != 1 || len(messenger.delivered) != 1 {
		t.Fatalf("rejected admission produced side effects: projection=%d messages=%d", len(projection.applied), len(messenger.delivered))
	}
}

func TestAcceptMatchRejectsStructurallyInvalidAdmission(t *testing.T) {
	tests := []struct {
		name      string
		admission matchentry.Admission
	}{
		{name: "missing MatchID", admission: matchentry.Admission{Teams: []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}}}},
		{name: "missing Teams", admission: matchentry.Admission{MatchID: "match-1"}},
		{name: "zero TeamID", admission: matchentry.Admission{MatchID: "match-1", Teams: []matchentry.MatchedTeam{{PlayerIDs: []int64{42}}}}},
		{name: "empty Team", admission: matchentry.Admission{MatchID: "match-1", Teams: []matchentry.MatchedTeam{{TeamID: 7}}}},
		{name: "zero PlayerID", admission: matchentry.Admission{MatchID: "match-1", Teams: []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{0}}}}},
		{name: "duplicate TeamID", admission: matchentry.Admission{MatchID: "match-1", Teams: []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}, {TeamID: 7, PlayerIDs: []int64{43}}}}},
		{name: "duplicate PlayerID", admission: matchentry.Admission{MatchID: "match-1", Teams: []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}, {TeamID: 8, PlayerIDs: []int64{42}}}}},
		{name: "only Robots", admission: matchentry.Admission{MatchID: "match-1", Teams: []matchentry.MatchedTeam{{TeamID: -1, PlayerIDs: []int64{-2}, IsRobot: true}}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projection := &fakeTeamRoomProjection{}
			messenger := &fakePlayerMessenger{}
			manager := newTestRoomManager(t,
				projection,
				messenger,
				func() time.Time { return time.Unix(100, 0) },
				func() string { return "room-invalid" },
			)

			result := manager.AcceptMatch(context.Background(), test.admission)

			if result.Status != matchentry.Rejected {
				t.Fatalf("invalid admission result = %#v", result)
			}
			if len(projection.applied) != 0 || len(messenger.delivered) != 0 {
				t.Fatalf("invalid admission produced side effects: projection=%d messages=%d", len(projection.applied), len(messenger.delivered))
			}
		})
	}
}

func TestAcceptMatchKeepsFailedTeamProjectionForReconciliation(t *testing.T) {
	projection := &fakeTeamRoomProjection{err: errors.New("projection unavailable")}
	messenger := &fakePlayerMessenger{}
	manager := newTestRoomManager(t,
		projection,
		messenger,
		func() time.Time { return time.Unix(100, 0) },
		func() string { return "room-1" },
	)

	result := manager.AcceptMatch(context.Background(), matchentry.Admission{
		MatchID: "match-1",
		Teams:   []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
	})
	if result.Status != matchentry.Accepted || len(messenger.delivered) != 1 {
		t.Fatalf("projection failure changed Room commit: result=%#v messages=%d", result, len(messenger.delivered))
	}

	projection.err = nil
	manager.ReconcileProjections()
	manager.ReconcileProjections()

	want := []participant.DesiredTeamRoom{{TeamID: 7, RoomID: "room-1"}}
	if len(projection.applied) != 2 || !slices.Equal(projection.applied[1], want) {
		t.Fatalf("projection attempts = %#v", projection.applied)
	}
}

func TestCloseRoomReleasesMembershipAndIsIdempotent(t *testing.T) {
	projection := &fakeTeamRoomProjection{}
	messenger := &fakePlayerMessenger{}
	roomIDs := []string{"room-1", "room-2"}
	manager := newTestRoomManager(t,
		projection,
		messenger,
		func() time.Time { return time.Unix(100, 0) },
		func() string {
			roomID := roomIDs[0]
			roomIDs = roomIDs[1:]
			return roomID
		},
	)
	firstAdmission := matchentry.Admission{
		MatchID: "match-1",
		Teams:   []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
	}
	manager.AcceptMatch(context.Background(), firstAdmission)

	if !manager.CloseRoom("room-1") || !manager.CloseRoom("room-1") {
		t.Fatal("CloseRoom() was not idempotent")
	}
	second := manager.AcceptMatch(context.Background(), matchentry.Admission{
		MatchID: "match-2",
		Teams:   []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
	})

	if second.Status != matchentry.Accepted || second.RoomID != "room-2" {
		t.Fatalf("admission after close = %#v", second)
	}
	wantProjection := [][]participant.DesiredTeamRoom{
		{{TeamID: 7, RoomID: "room-1"}},
		{{TeamID: 7, RoomID: ""}},
		{{TeamID: 7, RoomID: "room-2"}},
	}
	if len(projection.applied) != len(wantProjection) {
		t.Fatalf("projection history = %#v", projection.applied)
	}
	for index := range wantProjection {
		if !slices.Equal(projection.applied[index], wantProjection[index]) {
			t.Fatalf("projection[%d] = %#v", index, projection.applied[index])
		}
	}
}

func TestCloseRoomCoalescesPendingProjectionToLatestDesiredRoom(t *testing.T) {
	projection := &fakeTeamRoomProjection{err: errors.New("projection unavailable")}
	manager := newTestRoomManager(t,
		projection,
		&fakePlayerMessenger{},
		func() time.Time { return time.Unix(100, 0) },
		func() string { return "room-1" },
	)
	manager.AcceptMatch(context.Background(), matchentry.Admission{
		MatchID: "match-1",
		Teams:   []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
	})
	manager.CloseRoom("room-1")

	projection.err = nil
	manager.ReconcileProjections()
	manager.ReconcileProjections()

	want := []participant.DesiredTeamRoom{{TeamID: 7, RoomID: ""}}
	if len(projection.applied) != 3 || !slices.Equal(projection.applied[2], want) {
		t.Fatalf("projection attempts = %#v", projection.applied)
	}
}

func TestCloseRoomReappliesLatestProjectionAfterStaleJoinCompletes(t *testing.T) {
	projection := &reorderedTeamRoomProjection{
		joinStarted: make(chan struct{}),
		releaseJoin: make(chan struct{}),
	}
	manager := newTestRoomManager(t,
		projection,
		&fakePlayerMessenger{},
		func() time.Time { return time.Unix(100, 0) },
		func() string { return "room-1" },
	)
	accepted := make(chan matchentry.Acceptance, 1)
	go func() {
		accepted <- manager.AcceptMatch(context.Background(), matchentry.Admission{
			MatchID: "match-1",
			Teams:   []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
		})
	}()
	<-projection.joinStarted

	if !manager.CloseRoom("room-1") {
		t.Fatal("CloseRoom() = false")
	}
	close(projection.releaseJoin)
	select {
	case result := <-accepted:
		if result.Status != matchentry.Accepted {
			t.Fatalf("AcceptMatch() = %#v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("AcceptMatch did not finish")
	}

	history := projection.history()
	if len(history) != 3 || len(history[2]) != 1 || history[2][0].RoomID != "" {
		t.Fatalf("out-of-order projection history = %#v", history)
	}
}

func TestClosedMatchIDIsRejectedUntilTombstoneExpires(t *testing.T) {
	now := time.Unix(100, 0)
	roomIDs := []string{"room-1", "room-2"}
	manager := newTestRoomManager(t,
		&fakeTeamRoomProjection{},
		&fakePlayerMessenger{},
		func() time.Time { return now },
		func() string {
			roomID := roomIDs[0]
			roomIDs = roomIDs[1:]
			return roomID
		},
	)
	admission := matchentry.Admission{
		MatchID: "match-1",
		Teams:   []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
	}
	manager.AcceptMatch(context.Background(), admission)
	manager.CloseRoom("room-1")

	now = now.Add(4 * time.Minute)
	withinTombstone := manager.AcceptMatch(context.Background(), admission)
	now = now.Add(time.Minute)
	afterTombstone := manager.AcceptMatch(context.Background(), admission)

	if withinTombstone.Status != matchentry.Rejected || !slices.Equal(withinTombstone.RejectedTeamIDs, []int64{7}) {
		t.Fatalf("within tombstone = %#v", withinTombstone)
	}
	if afterTombstone.Status != matchentry.Accepted || afterTombstone.RoomID != "room-2" {
		t.Fatalf("after tombstone = %#v", afterTombstone)
	}
}

func TestMaintainProactivelyPrunesExpiredMatchTombstones(t *testing.T) {
	now := time.Unix(100, 0)
	manager := newTestRoomManager(t,
		&fakeTeamRoomProjection{},
		&fakePlayerMessenger{},
		func() time.Time { return now },
		func() string { return "room-1" },
	)
	manager.AcceptMatch(context.Background(), matchentry.Admission{
		MatchID: "match-1",
		Teams:   []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
	})
	manager.CloseRoom("room-1")

	now = now.Add(MatchTombstoneTTL - time.Nanosecond)
	manager.Maintain()
	if len(manager.closedMatches) != 1 {
		t.Fatalf("到期前 tombstones = %#v", manager.closedMatches)
	}
	if !manager.CloseRoom("room-1") {
		t.Fatal("到期前重复关闭应保持幂等")
	}

	now = now.Add(time.Nanosecond)
	manager.Maintain()
	if len(manager.closedMatches) != 0 {
		t.Fatalf("到期时 tombstones = %#v", manager.closedMatches)
	}
	if len(manager.closedRooms) != 0 || manager.CloseRoom("room-1") {
		t.Fatalf("到期后仍保留已完成的关闭记录: %#v", manager.closedRooms)
	}
}

func TestMaintainRetainsPendingRoomCleanupPastTombstoneTTL(t *testing.T) {
	for _, pending := range []string{"projection", "actor stop"} {
		t.Run(pending, func(t *testing.T) {
			now := time.Unix(100, 0)
			projection := &fakeTeamRoomProjection{}
			manager := newTestRoomManager(t, projection, &fakePlayerMessenger{}, func() time.Time { return now }, func() string { return "room-1" })
			manager.AcceptMatch(context.Background(), matchentry.Admission{
				MatchID: "match-1",
				Teams:   []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
			})
			ctx := context.Background()
			wantErr := errors.New("projection unavailable")
			if pending == "projection" {
				projection.err = wantErr
			} else {
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				ctx, wantErr = canceled, context.Canceled
			}
			if closed, err := manager.closeRoom(ctx, "room-1", false); !closed || !errors.Is(err, wantErr) {
				t.Fatalf("first close = %t, %v; want %v", closed, err, wantErr)
			}

			now = now.Add(MatchTombstoneTTL)
			manager.Maintain()
			if _, retained := manager.closedRooms["room-1"]; !retained {
				t.Fatal("墓碑到期不应丢弃未完成的房间清理")
			}
			if pending == "actor stop" {
				if manager.projectedTeamRoomVersions[7] != manager.desiredTeamRooms[7].version {
					t.Fatal("Actor 停止重试不应阻塞投影补交")
				}
				if _, err := manager.closeRoom(context.Background(), "room-1", false); err != nil {
					t.Fatalf("retry close = %v", err)
				}
			}
			projection.err = nil
			manager.Maintain()
			if _, retained := manager.closedRooms["room-1"]; retained {
				t.Fatal("清理完成后应回收已到期的房间关闭记录")
			}
		})
	}
}

func TestHandleRecordOperateAuthorizesCanonicalMembership(t *testing.T) {
	messenger := &fakePlayerMessenger{}
	manager := newTestRoomManager(t,
		&fakeTeamRoomProjection{},
		messenger,
		func() time.Time { return time.Unix(100, 0) },
		func() string { return "room-1" },
	)
	manager.AcceptMatch(context.Background(), matchentry.Admission{
		MatchID: "match-1",
		Teams: []matchentry.MatchedTeam{
			{TeamID: 7, PlayerIDs: []int64{42, 43}},
			{TeamID: -1, PlayerIDs: []int64{-2}, IsRobot: true},
		},
	})
	messenger.delivered = nil

	valid := manager.HandleRecordOperate(42, "room-1", "move-left")
	invalid := manager.HandleRecordOperate(42, "room-other", "move-right")

	if valid.OperateInfo != "move-left" || invalid.OperateInfo != "" {
		t.Fatalf("responses = valid:%#v invalid:%#v", valid, invalid)
	}
	if len(messenger.delivered) != 1 {
		t.Fatalf("broadcasts = %#v", messenger.delivered)
	}
	delivery := messenger.delivered[0]
	if !slices.Equal(delivery.playerIDs, []int64{42, 43}) || delivery.excluded != 42 {
		t.Fatalf("broadcast recipients = %#v", delivery)
	}
	broadcast, ok := delivery.msg.(*message.S2C_RecordGameOperate)
	if !ok || broadcast.OperateInfo != "move-left" {
		t.Fatalf("broadcast = %#v", delivery.msg)
	}
}

func TestPlayerOfflineNotifiesOthersWithoutRemovingMembership(t *testing.T) {
	messenger := &fakePlayerMessenger{}
	manager := newTestRoomManager(t,
		&fakeTeamRoomProjection{},
		messenger,
		func() time.Time { return time.Unix(100, 0) },
		func() string { return "room-1" },
	)
	manager.AcceptMatch(context.Background(), matchentry.Admission{
		MatchID: "match-1",
		Teams: []matchentry.MatchedTeam{
			{TeamID: 7, PlayerIDs: []int64{42, 43}},
			{TeamID: -1, PlayerIDs: []int64{-2}, IsRobot: true},
		},
	})
	messenger.delivered = nil

	notified := manager.PlayerOffline(context.Background(), 42)
	unknown := manager.PlayerOffline(context.Background(), 99)
	operation := manager.HandleRecordOperate(42, "room-1", "reconnected-move")

	if !notified || unknown || operation.OperateInfo != "reconnected-move" {
		t.Fatalf("offline results = notified:%t unknown:%t operation:%#v", notified, unknown, operation)
	}
	if len(messenger.delivered) != 2 {
		t.Fatalf("deliveries = %#v", messenger.delivered)
	}
	offlineDelivery := messenger.delivered[0]
	if !slices.Equal(offlineDelivery.playerIDs, []int64{42, 43}) || offlineDelivery.excluded != 42 {
		t.Fatalf("offline recipients = %#v", offlineDelivery)
	}
	offline, ok := offlineDelivery.msg.(*message.S2C_PlayerOffline)
	if !ok || offline.PlayerId != "42" {
		t.Fatalf("offline message = %#v", offlineDelivery.msg)
	}
}

func TestCloseExpiredClosesOnlyRoomsAtMaximumLifetime(t *testing.T) {
	createdAt := time.Unix(100, 0)
	now := createdAt
	roomNumber := 0
	manager := newTestRoomManager(t,
		&fakeTeamRoomProjection{},
		&fakePlayerMessenger{},
		func() time.Time { return now },
		func() string {
			roomNumber++
			return "room-" + strconv.Itoa(roomNumber)
		},
	)
	manager.AcceptMatch(context.Background(), matchentry.Admission{
		MatchID: "match-1",
		Teams:   []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
	})
	now = now.Add(time.Minute)
	manager.AcceptMatch(context.Background(), matchentry.Admission{
		MatchID: "match-2",
		Teams:   []matchentry.MatchedTeam{{TeamID: 8, PlayerIDs: []int64{43}}},
	})

	now = createdAt.Add(MaxRoomLifetime)
	closed := manager.CloseExpired()
	firstTeam := manager.AcceptMatch(context.Background(), matchentry.Admission{
		MatchID: "match-3",
		Teams:   []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
	})
	secondTeam := manager.AcceptMatch(context.Background(), matchentry.Admission{
		MatchID: "match-4",
		Teams:   []matchentry.MatchedTeam{{TeamID: 8, PlayerIDs: []int64{43}}},
	})

	if closed != 1 || firstTeam.Status != matchentry.Accepted || secondTeam.Status != matchentry.Rejected {
		t.Fatalf("expiry results = closed:%d first:%#v second:%#v", closed, firstTeam, secondTeam)
	}
}

func TestRoomManagerStopClosesOwnedRoomsWithoutStoppingOtherActors(t *testing.T) {
	system := actor.NewActorSystem(time.Second)
	roomScope, err := system.NewScope("room-test")
	if err != nil {
		t.Fatalf("NewScope room: %v", err)
	}
	otherScope, err := system.NewScope("other-test")
	if err != nil {
		t.Fatalf("NewScope other: %v", err)
	}
	t.Cleanup(func() {
		if err := system.Stop(context.Background()); err != nil {
			t.Errorf("Stop ActorSystem: %v", err)
		}
	})
	otherDefinition, err := actor.Define(otherScope, func(context.Context, string) (*unrelatedRoomTestActor, error) {
		return &unrelatedRoomTestActor{}, nil
	}, actor.DefinitionOptions{QueueCapacity: 1024})
	if err != nil {
		t.Fatalf("Define unrelated actor: %v", err)
	}
	unrelated, err := otherDefinition.GetOrCreate(context.Background(), "other")
	if err != nil {
		t.Fatalf("create unrelated actor: %v", err)
	}
	projection := &fakeTeamRoomProjection{}
	manager, err := NewRoomManager(roomScope,
		projection,
		&fakePlayerMessenger{},
		func() time.Time { return time.Unix(100, 0) },
		func() string { return "room-1" },
	)
	if err != nil {
		t.Fatalf("NewRoomManager: %v", err)
	}
	manager.AcceptMatch(context.Background(), matchentry.Admission{
		MatchID: "match-1",
		Teams:   []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
	})

	manager.Stop(context.Background())
	manager.Stop(context.Background())

	if _, err := actor.Command(context.Background(), unrelated.Ref(), func(actor.Context) (struct{}, error) {
		return struct{}{}, nil
	}); err != nil {
		t.Fatalf("RoomManager.Stop affected unrelated actor: %v", err)
	}
	wantClear := []participant.DesiredTeamRoom{{TeamID: 7, RoomID: ""}}
	if len(projection.applied) != 2 || !slices.Equal(projection.applied[1], wantClear) {
		t.Fatalf("Room shutdown projections = %#v", projection.applied)
	}
}

func TestNewRoomManagerRejectsMissingDependency(t *testing.T) {
	tests := []struct {
		name       string
		projection participant.TeamRoomProjection
		messenger  participant.PlayerMessenger
		now        func() time.Time
		newRoomID  func() string
	}{
		{name: "projection", messenger: &fakePlayerMessenger{}, now: time.Now, newRoomID: func() string { return "room-1" }},
		{name: "messenger", projection: &fakeTeamRoomProjection{}, now: time.Now, newRoomID: func() string { return "room-1" }},
		{name: "clock", projection: &fakeTeamRoomProjection{}, messenger: &fakePlayerMessenger{}, newRoomID: func() string { return "room-1" }},
		{name: "RoomID generator", projection: &fakeTeamRoomProjection{}, messenger: &fakePlayerMessenger{}, now: time.Now},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			system := actor.NewActorSystem(time.Second)
			scope, err := system.NewScope("room-test")
			if err != nil {
				t.Fatalf("NewScope: %v", err)
			}
			if _, err := NewRoomManager(scope, test.projection, test.messenger, test.now, test.newRoomID); err == nil {
				t.Fatal("NewRoomManager accepted missing dependency")
			}
		})
	}
}

func TestRecordOperateRejectsStoppedRoom(t *testing.T) {
	manager := newTestRoomManager(t, &fakeTeamRoomProjection{}, &fakePlayerMessenger{}, time.Now, func() string { return "room-1" })
	manager.AcceptMatch(context.Background(), matchentry.Admission{
		MatchID: "match-1", Teams: []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
	})
	if err := manager.rooms["room-1"].Ref().Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if response := manager.HandleRecordOperate(42, "room-1", "move"); response.OperateInfo != "" {
		t.Fatalf("stopped Room acknowledged an unapplied operation: %#v", response)
	}
}

func TestRoomManagerStopRespectsDeadlineAndRejectsLaterMatches(t *testing.T) {
	manager := newTestRoomManager(t, &fakeTeamRoomProjection{}, &fakePlayerMessenger{}, time.Now, func() string { return "room-1" })
	manager.AcceptMatch(context.Background(), matchentry.Admission{
		MatchID: "match-1", Teams: []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
	})
	room := manager.rooms["room-1"]
	started, release := make(chan struct{}), make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseOnce)
	if err := actor.Tell(context.Background(), room.Ref(), func(actor.Context) error {
		close(started)
		<-release
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := manager.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop while Room is busy = %v", err)
	}
	result := manager.AcceptMatch(context.Background(), matchentry.Admission{
		MatchID: "match-2", Teams: []matchentry.MatchedTeam{{TeamID: 8, PlayerIDs: []int64{43}}},
	})
	if result.Status != matchentry.Retryable {
		t.Fatalf("match after Stop = %#v", result)
	}
	releaseOnce()
	if err := room.Ref().Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRoomManagerStopRetriesPendingProjection(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(strconv.FormatBool(canceled), func(t *testing.T) {
			projection := &fakeTeamRoomProjection{}
			manager := newTestRoomManager(t, projection, &fakePlayerMessenger{}, time.Now, func() string { return "room-1" })
			manager.AcceptMatch(context.Background(), matchentry.Admission{
				MatchID: "match-1", Teams: []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("projection unavailable")
			if canceled {
				cancel()
				failure = context.Canceled
			} else {
				projection.err = failure
			}
			if err := manager.Stop(ctx); !errors.Is(err, failure) {
				t.Fatalf("first Stop = %v; want %v", err, failure)
			}
			beforeRetry := len(projection.applied)
			projection.err = nil
			if err := manager.Stop(context.Background()); err != nil {
				t.Fatalf("retry Stop = %v", err)
			}
			if len(projection.applied) != beforeRetry+1 {
				t.Fatalf("retry did not apply the pending projection: before=%d after=%d", beforeRetry, len(projection.applied))
			}
			last := projection.applied[len(projection.applied)-1]
			if !slices.Equal(last, []participant.DesiredTeamRoom{{TeamID: 7, RoomID: ""}}) {
				t.Fatalf("retry left stale RoomID: %#v", last)
			}
			applied := len(projection.applied)
			if err := manager.Stop(context.Background()); err != nil || len(projection.applied) != applied {
				t.Fatalf("completed cleanup was repeated: err=%v calls=%d", err, len(projection.applied))
			}
		})
	}
}
