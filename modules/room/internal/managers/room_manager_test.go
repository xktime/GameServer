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

type reorderedTeamRoomProjection struct {
	joinStarted chan struct{}
	releaseJoin chan struct{}
	joinOnce    sync.Once
	mu          sync.Mutex
	applied     [][]participant.DesiredTeamRoom
}

func (p *reorderedTeamRoomProjection) Apply(desired []participant.DesiredTeamRoom) error {
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

func (f *fakeTeamRoomProjection) Apply(desired []participant.DesiredTeamRoom) error {
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

func TestAcceptMatchCommitsRoomAndNotifiesOnlyRealPlayers(t *testing.T) {
	projection := &fakeTeamRoomProjection{}
	messenger := &fakePlayerMessenger{}
	manager := newTestRoomManager(t,
		projection,
		messenger,
		func() time.Time { return time.Unix(100, 0) },
		func() string { return "room-1" },
	)

	result := manager.AcceptMatch(matchentry.Admission{
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

	first := manager.AcceptMatch(admission)
	second := manager.AcceptMatch(admission)

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
	manager.AcceptMatch(matchentry.Admission{
		MatchID: "match-1",
		Teams:   []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
	})

	result := manager.AcceptMatch(matchentry.Admission{
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

			result := manager.AcceptMatch(test.admission)

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

	result := manager.AcceptMatch(matchentry.Admission{
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
	manager.AcceptMatch(firstAdmission)

	if !manager.CloseRoom("room-1") || !manager.CloseRoom("room-1") {
		t.Fatal("CloseRoom() was not idempotent")
	}
	second := manager.AcceptMatch(matchentry.Admission{
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
	manager.AcceptMatch(matchentry.Admission{
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
		accepted <- manager.AcceptMatch(matchentry.Admission{
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
	manager.AcceptMatch(admission)
	manager.CloseRoom("room-1")

	now = now.Add(4 * time.Minute)
	withinTombstone := manager.AcceptMatch(admission)
	now = now.Add(time.Minute)
	afterTombstone := manager.AcceptMatch(admission)

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
	manager.AcceptMatch(matchentry.Admission{
		MatchID: "match-1",
		Teams:   []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
	})
	manager.CloseRoom("room-1")

	now = now.Add(MatchTombstoneTTL - time.Nanosecond)
	manager.Maintain()
	if len(manager.closedMatches) != 1 {
		t.Fatalf("到期前 tombstones = %#v", manager.closedMatches)
	}

	now = now.Add(time.Nanosecond)
	manager.Maintain()
	if len(manager.closedMatches) != 0 {
		t.Fatalf("到期时 tombstones = %#v", manager.closedMatches)
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
	manager.AcceptMatch(matchentry.Admission{
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
	manager.AcceptMatch(matchentry.Admission{
		MatchID: "match-1",
		Teams: []matchentry.MatchedTeam{
			{TeamID: 7, PlayerIDs: []int64{42, 43}},
			{TeamID: -1, PlayerIDs: []int64{-2}, IsRobot: true},
		},
	})
	messenger.delivered = nil

	notified := manager.PlayerOffline(42)
	unknown := manager.PlayerOffline(99)
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
	manager.AcceptMatch(matchentry.Admission{
		MatchID: "match-1",
		Teams:   []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
	})
	now = now.Add(time.Minute)
	manager.AcceptMatch(matchentry.Admission{
		MatchID: "match-2",
		Teams:   []matchentry.MatchedTeam{{TeamID: 8, PlayerIDs: []int64{43}}},
	})

	now = createdAt.Add(MaxRoomLifetime)
	closed := manager.CloseExpired()
	firstTeam := manager.AcceptMatch(matchentry.Admission{
		MatchID: "match-3",
		Teams:   []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
	})
	secondTeam := manager.AcceptMatch(matchentry.Admission{
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
	otherDefinition, err := actor.Define(otherScope, actor.Test1, func(context.Context, string) (*unrelatedRoomTestActor, error) {
		return &unrelatedRoomTestActor{}, nil
	})
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
	manager.AcceptMatch(matchentry.Admission{
		MatchID: "match-1",
		Teams:   []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
	})

	manager.Stop()
	manager.Stop()

	if _, err := actor.Call(context.Background(), unrelated.Ref(), func(actor.Context) (struct{}, error) {
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
