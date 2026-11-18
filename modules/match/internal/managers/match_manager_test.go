package managers

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	"gameserver/modules/match/playerread"
	"gameserver/modules/room/matchentry"
	"reflect"
	"slices"
	"testing"
	"time"
)

type fakeMatchPlayerReader struct {
	players map[int64]playerread.PlayerSnapshot
	teams   map[int64]playerread.TeamSnapshot
	err     error
}

func (f *fakeMatchPlayerReader) FindOnline(ctx context.Context, playerID int64) (playerread.PlayerSnapshot, bool, error) {
	if f.err != nil {
		return playerread.PlayerSnapshot{}, false, f.err
	}
	snapshot, ok := f.players[playerID]
	return snapshot, ok, nil
}

func (f *fakeMatchPlayerReader) FindOnlineTeam(ctx context.Context, playerID int64) (playerread.TeamSnapshot, bool, error) {
	if f.err != nil {
		return playerread.TeamSnapshot{}, false, f.err
	}
	snapshot, ok := f.teams[playerID]
	snapshot.MemberIDs = append([]int64(nil), snapshot.MemberIDs...)
	return snapshot, ok, nil
}

func TestMatchManagerPropagatesLookupFailuresWithoutChangingQueue(t *testing.T) {
	for _, failure := range []error{context.Canceled, context.DeadlineExceeded, actor.ErrActorQuarantined, errors.New("storage unavailable")} {
		t.Run(failure.Error(), func(t *testing.T) {
			players := &fakeMatchPlayerReader{err: failure}
			manager := newMatchTestManager(players)
			manager.matchQueues[1].AddTeamRequest(&teamMatchRequest{TeamId: 7, PlayerIds: []int64{42}})
			if _, response, err := manager.doHandleMatch(context.Background(), 42, &message.C2S_StartMatch{Type: 1}); response != nil || !errors.Is(err, failure) {
				t.Fatalf("start match lookup failure = %v, %v", response, err)
			}
			if _, response, err := manager.doHandleCancelMatch(context.Background(), 42); response != nil || !errors.Is(err, failure) {
				t.Fatalf("cancel match lookup failure = %v, %v", response, err)
			}
			if manager.matchQueues[1].GetQueueSize() != 1 || !manager.matchQueues[1].IsTeamInQueue(7) {
				t.Fatal("failed lookup changed the matching queue")
			}
		})
	}
}

func newMatchTestManager(players playerread.PlayerReader) *matchActor {
	return &matchActor{
		players: players,
		now:     time.Now,
		matchQueues: map[int32]*matchQueue{
			1: newMatchQueue(),
		},
	}
}

func TestMatchManagerStartsAndCancelsMatchFromSnapshots(t *testing.T) {
	players := &fakeMatchPlayerReader{
		teams: map[int64]playerread.TeamSnapshot{42: {TeamID: 7, MemberIDs: []int64{42, 43}}},
	}
	manager := newMatchTestManager(players)
	teamID, response, err := manager.doHandleMatch(context.Background(), 42, &message.C2S_StartMatch{Type: 1})
	if err != nil || response == nil || !response.Result || teamID != 7 {
		t.Fatalf("开始匹配结果 = %d, %#v", teamID, response)
	}
	request := manager.matchQueues[1].TeamRequests[7]
	if request == nil || !slices.Equal(request.PlayerIds, []int64{42, 43}) {
		t.Fatalf("匹配队列请求 = %#v", request)
	}
	duplicateTeamID, duplicateResponse, err := manager.doHandleMatch(context.Background(), 42, &message.C2S_StartMatch{Type: 1})
	if err != nil || duplicateResponse == nil || duplicateResponse.Result || duplicateTeamID != 0 || manager.matchQueues[1].GetQueueSize() != 1 {
		t.Fatalf("重复开始匹配结果 = %d, %#v", duplicateTeamID, duplicateResponse)
	}

	players.players = map[int64]playerread.PlayerSnapshot{42: {TeamID: 7}}
	teamID, cancelResponse, err := manager.doHandleCancelMatch(context.Background(), 42)
	if err != nil || cancelResponse == nil || !cancelResponse.Result || teamID != 7 {
		t.Fatalf("取消匹配结果 = %d, %#v", teamID, cancelResponse)
	}
	if manager.matchQueues[1].IsTeamInQueue(7) {
		t.Fatal("取消后 Team 不应留在匹配队列")
	}
}

func TestMatchManagerUsesInjectedClockForQueueAndTimeout(t *testing.T) {
	now := time.Date(2026, time.August, 23, 12, 0, 0, 0, time.UTC)
	players := &fakeMatchPlayerReader{
		teams: map[int64]playerread.TeamSnapshot{42: {TeamID: 7, MemberIDs: []int64{42}}},
	}
	manager := newMatchTestManager(players)
	manager.now = func() time.Time { return now }

	teamID, response, err := manager.doHandleMatch(context.Background(), 42, &message.C2S_StartMatch{Type: 1})
	request := manager.matchQueues[1].TeamRequests[7]
	if err != nil || response == nil || !response.Result || teamID != 7 || request == nil {
		t.Fatalf("开始匹配结果 = %d, %#v，请求 = %#v", teamID, response, request)
	}
	if !request.JoinTime.Equal(now) {
		t.Fatalf("加入时间 = %s，期望注入时间 %s", request.JoinTime, now)
	}

	now = now.Add(5*time.Minute + time.Nanosecond)
	manager.processTimeoutRequests()
	if manager.matchQueues[1].IsTeamInQueue(7) {
		t.Fatal("超过五分钟的请求应由注入时钟判定为过期")
	}
}

func TestMatchManagerRejectsUnavailableParticipants(t *testing.T) {
	tests := []struct {
		name      string
		players   *fakeMatchPlayerReader
		matchType int32
	}{
		{
			name:      "offline player",
			players:   &fakeMatchPlayerReader{},
			matchType: 1,
		},
		{
			name: "player without team",
			players: &fakeMatchPlayerReader{
				teams: map[int64]playerread.TeamSnapshot{42: {MemberIDs: []int64{42}}},
			},
			matchType: 1,
		},
		{
			name: "missing team snapshot",
			players: &fakeMatchPlayerReader{
				players: map[int64]playerread.PlayerSnapshot{42: {TeamID: 7}},
			},
			matchType: 1,
		},
		{
			name: "invalid match type",
			players: &fakeMatchPlayerReader{
				players: map[int64]playerread.PlayerSnapshot{42: {TeamID: 7}},
				teams:   map[int64]playerread.TeamSnapshot{42: {TeamID: 7, MemberIDs: []int64{42}}},
			},
			matchType: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := newMatchTestManager(test.players)
			teamID, response, err := manager.doHandleMatch(context.Background(), 42, &message.C2S_StartMatch{Type: test.matchType})
			if err != nil || response == nil || response.Result || teamID != 0 {
				t.Fatalf("不可用参与者的开始匹配结果 = %d, %#v", teamID, response)
			}
			if manager.matchQueues[1].GetQueueSize() != 0 {
				t.Fatal("失败的开始匹配不应改写队列")
			}
		})
	}
}

func TestMatchManagerBuildsSyntheticRobotTeams(t *testing.T) {
	now := time.Date(2026, time.August, 23, 12, 0, 0, 0, time.UTC)
	manager := newMatchTestManager(&fakeMatchPlayerReader{})
	manager.now = func() time.Time { return now }

	teams := manager.syntheticRobotTeams(2)
	if len(teams) != 2 {
		t.Fatalf("机器人 Team 数量 = %d，期望 2", len(teams))
	}
	if !slices.Equal(teams[0].PlayerIds, []int64{-2}) || !slices.Equal(teams[1].PlayerIds, []int64{-4}) {
		t.Fatalf("机器人 Player = %#v", teams)
	}
	if !teams[0].IsRobot || !teams[1].IsRobot || teams[0].TeamId != -1 || teams[1].TeamId != -3 {
		t.Fatalf("机器人 Team 属性不正确: %#v", teams)
	}
	if !teams[0].JoinTime.Equal(now) || !teams[1].JoinTime.Equal(now) {
		t.Fatalf("机器人加入时间未使用注入时钟: %#v", teams)
	}
}

type fakeRoomAcceptor struct {
	admissions []matchentry.Admission
	responses  []matchentry.Acceptance
}

func (f *fakeRoomAcceptor) AcceptMatch(ctx context.Context, admission matchentry.Admission) matchentry.Acceptance {
	copied := matchentry.Admission{MatchID: admission.MatchID, Teams: make([]matchentry.MatchedTeam, len(admission.Teams))}
	for index, team := range admission.Teams {
		copied.Teams[index] = team
		copied.Teams[index].PlayerIDs = append([]int64(nil), team.PlayerIDs...)
	}
	f.admissions = append(f.admissions, copied)
	response := f.responses[0]
	f.responses = f.responses[1:]
	return response
}

func TestMatchingRetriesSameAdmissionAfterRetryable(t *testing.T) {
	rooms := &fakeRoomAcceptor{responses: []matchentry.Acceptance{
		{Status: matchentry.Retryable},
		{Status: matchentry.AlreadyAccepted, RoomID: "room-1"},
	}}
	matchIDCalls := 0
	manager := newMatchTestManager(&fakeMatchPlayerReader{
		players: map[int64]playerread.PlayerSnapshot{42: {TeamID: 7}},
	})
	manager.rooms = rooms
	manager.newMatchID = func() string {
		matchIDCalls++
		return "match-1"
	}
	manager.settlements = make(map[string]*matchSettlement)
	manager.matchQueues[1].AddTeamRequest(&teamMatchRequest{
		TeamId: 7, PlayerIds: []int64{42},
	})

	manager.matching(context.Background())
	request := manager.matchQueues[1].TeamRequests[7]
	canceledTeamID, cancelResponse, err := manager.doHandleCancelMatch(context.Background(), 42)
	if err != nil || request == nil || request.MatchID != "match-1" || manager.matchQueues[1].RemoveTeamRequest(7) || canceledTeamID != 0 || cancelResponse.Result {
		t.Fatalf("retryable settlement state = %#v", request)
	}
	if len(rooms.admissions) != 1 || len(rooms.admissions[0].Teams) != 5 {
		t.Fatalf("first admission = %#v", rooms.admissions)
	}
	manager.processTimeoutRequests()
	if request = manager.matchQueues[1].TeamRequests[7]; request == nil || request.MatchID != "match-1" {
		t.Fatalf("timeout removed settling request: %#v", request)
	}

	manager.matching(context.Background())
	if len(rooms.admissions) != 2 || !reflect.DeepEqual(rooms.admissions[0], rooms.admissions[1]) {
		t.Fatalf("retried admissions = %#v", rooms.admissions)
	}
	if matchIDCalls != 1 || manager.matchQueues[1].IsTeamInQueue(7) {
		t.Fatalf("resolved state = MatchID calls:%d requests:%#v", matchIDCalls, manager.matchQueues[1].TeamRequests)
	}
}

func TestMatchingRemovesRejectedTeamsAndRequeuesTheRest(t *testing.T) {
	rooms := &fakeRoomAcceptor{responses: []matchentry.Acceptance{{
		Status:          matchentry.Rejected,
		RejectedTeamIDs: []int64{7, -1, 999},
	}}}
	manager := newMatchTestManager(&fakeMatchPlayerReader{})
	manager.rooms = rooms
	manager.newMatchID = func() string { return "match-1" }
	manager.settlements = make(map[string]*matchSettlement)
	queue := manager.matchQueues[1]
	queue.AddTeamRequest(&teamMatchRequest{
		TeamId: 7, PlayerIds: []int64{42, 43},
	})
	queue.AddTeamRequest(&teamMatchRequest{
		TeamId: 8, PlayerIds: []int64{44},
	})

	manager.matching(context.Background())

	if queue.IsTeamInQueue(7) {
		t.Fatalf("rejected Team remained queued: requests=%#v", queue.TeamRequests)
	}
	request := queue.TeamRequests[8]
	if request == nil || request.MatchID != "" {
		t.Fatalf("non-rejected Team was not requeued: %#v", request)
	}
	if len(manager.settlements) != 0 || len(rooms.admissions) != 1 {
		t.Fatalf("settlement state = %#v, admissions=%d", manager.settlements, len(rooms.admissions))
	}
}

func TestNewMatchManagerRegistersReadyActorInScope(t *testing.T) {
	system := actor.NewActorSystem(time.Second)
	scope, err := system.NewScope("match-test")
	if err != nil {
		t.Fatalf("NewScope: %v", err)
	}
	players := &fakeMatchPlayerReader{}
	rooms := &fakeRoomAcceptor{}
	newMatchID := func() string { return "match-1" }
	now := func() time.Time { return time.Unix(1, 0) }
	manager, err := NewMatchManager(context.Background(), scope, players, rooms, newMatchID, now)
	if err != nil {
		t.Fatalf("NewMatchManager: %v", err)
	}

	if manager.actor.players != players || manager.actor.rooms != rooms || manager.actor.newMatchID == nil || manager.actor.now == nil || manager.actor.matchQueues[1] == nil || manager.actor.settlements == nil {
		t.Fatalf("initialized manager = %#v", manager)
	}
	if _, err := actor.Command(context.Background(), manager.actor.Ref(), func(actor.Context) (struct{}, error) {
		return struct{}{}, nil
	}); err != nil {
		t.Fatalf("Call ready manager: %v", err)
	}
}
