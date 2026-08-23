package managers

import (
	commonmodels "gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	matchmodels "gameserver/modules/match/internal/models"
	"gameserver/modules/match/playerread"
	"gameserver/modules/match/roomaccept"
	"gameserver/modules/room/matchentry"
	"net"
	"reflect"
	"slices"
	"testing"
	"time"
)

type fakeMatchAgent struct {
	userData any
}

var _ gate.Agent = (*fakeMatchAgent)(nil)

func (a *fakeMatchAgent) WriteMsg(any)                {}
func (a *fakeMatchAgent) WriteMsgWithSeq(any, uint32) {}
func (a *fakeMatchAgent) LocalAddr() net.Addr         { return nil }
func (a *fakeMatchAgent) RemoteAddr() net.Addr        { return nil }
func (a *fakeMatchAgent) Close()                      {}
func (a *fakeMatchAgent) Destroy()                    {}
func (a *fakeMatchAgent) UserData() any               { return a.userData }
func (a *fakeMatchAgent) SetUserData(userData any)    { a.userData = userData }

type fakeMatchPlayerReader struct {
	players map[int64]playerread.PlayerSnapshot
	teams   map[int64]playerread.TeamSnapshot
}

func (f *fakeMatchPlayerReader) FindOnline(playerID int64) (playerread.PlayerSnapshot, bool) {
	snapshot, ok := f.players[playerID]
	return snapshot, ok
}

func (f *fakeMatchPlayerReader) FindOnlineTeam(playerID int64) (playerread.TeamSnapshot, bool) {
	snapshot, ok := f.teams[playerID]
	snapshot.MemberIDs = append([]int64(nil), snapshot.MemberIDs...)
	return snapshot, ok
}

func newMatchTestManager(players playerread.PlayerReader) *MatchManager {
	return &MatchManager{
		players: players,
		now:     time.Now,
		matchQueues: map[int32]*matchmodels.MatchQueue{
			1: matchmodels.NewMatchQueue(),
		},
	}
}

func matchTestAgent(playerID int64) gate.Agent {
	return &fakeMatchAgent{userData: commonmodels.User{PlayerId: playerID}}
}

func TestMatchManagerStartsAndCancelsMatchFromSnapshots(t *testing.T) {
	players := &fakeMatchPlayerReader{
		players: map[int64]playerread.PlayerSnapshot{42: {TeamID: 7}},
		teams:   map[int64]playerread.TeamSnapshot{42: {MemberIDs: []int64{42, 43}}},
	}
	manager := newMatchTestManager(players)
	agent := matchTestAgent(42)

	teamID, response := manager.doHandleMatch(agent, &message.C2S_StartMatch{Type: 1})
	if response == nil || !response.Result || teamID != 7 {
		t.Fatalf("开始匹配结果 = %d, %#v", teamID, response)
	}
	request := manager.matchQueues[1].TeamRequests[7]
	if request == nil || !slices.Equal(request.PlayerIds, []int64{42, 43}) || request.TeamSize != 2 {
		t.Fatalf("匹配队列请求 = %#v", request)
	}
	duplicateTeamID, duplicateResponse := manager.doHandleMatch(agent, &message.C2S_StartMatch{Type: 1})
	if duplicateResponse == nil || duplicateResponse.Result || duplicateTeamID != 0 || manager.matchQueues[1].GetQueueSize() != 1 {
		t.Fatalf("重复开始匹配结果 = %d, %#v", duplicateTeamID, duplicateResponse)
	}

	teamID, cancelResponse := manager.doHandleCancelMatch(agent)
	if cancelResponse == nil || !cancelResponse.Result || teamID != 7 {
		t.Fatalf("取消匹配结果 = %d, %#v", teamID, cancelResponse)
	}
	if manager.matchQueues[1].IsTeamInQueue(7) {
		t.Fatal("取消后 Team 不应留在匹配队列")
	}
}

func TestMatchManagerUsesInjectedClockForQueueAndTimeout(t *testing.T) {
	now := time.Date(2026, time.August, 23, 12, 0, 0, 0, time.UTC)
	players := &fakeMatchPlayerReader{
		players: map[int64]playerread.PlayerSnapshot{42: {TeamID: 7}},
		teams:   map[int64]playerread.TeamSnapshot{42: {MemberIDs: []int64{42}}},
	}
	manager := newMatchTestManager(players)
	manager.now = func() time.Time { return now }

	teamID, response := manager.doHandleMatch(matchTestAgent(42), &message.C2S_StartMatch{Type: 1})
	request := manager.matchQueues[1].TeamRequests[7]
	if response == nil || !response.Result || teamID != 7 || request == nil {
		t.Fatalf("开始匹配结果 = %d, %#v，请求 = %#v", teamID, response, request)
	}
	if !request.JoinTime.Equal(now) {
		t.Fatalf("加入时间 = %s，期望注入时间 %s", request.JoinTime, now)
	}

	now = now.Add(5*time.Minute + time.Nanosecond)
	manager.ProcessTimeoutRequests()
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
				players: map[int64]playerread.PlayerSnapshot{42: {}},
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
				teams:   map[int64]playerread.TeamSnapshot{42: {MemberIDs: []int64{42}}},
			},
			matchType: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := newMatchTestManager(test.players)
			teamID, response := manager.doHandleMatch(matchTestAgent(42), &message.C2S_StartMatch{Type: test.matchType})
			if response == nil || response.Result || teamID != 0 {
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

	teams := manager.syntheticRobotTeams(1, 2)
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

func (f *fakeRoomAcceptor) AcceptMatch(admission matchentry.Admission) matchentry.Acceptance {
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
	manager.matchQueues[1].AddTeamRequest(&matchmodels.TeamMatchRequest{
		TeamId: 7, PlayerIds: []int64{42}, TeamSize: 1, MatchType: 1,
	})

	manager.Matching()
	request := manager.matchQueues[1].TeamRequests[7]
	canceledTeamID, cancelResponse := manager.doHandleCancelMatch(matchTestAgent(42))
	if request == nil || request.MatchID != "match-1" || manager.matchQueues[1].RemoveTeamRequest(7) || canceledTeamID != 0 || cancelResponse.Result {
		t.Fatalf("retryable settlement state = %#v", request)
	}
	if len(rooms.admissions) != 1 || len(rooms.admissions[0].Teams) != 5 {
		t.Fatalf("first admission = %#v", rooms.admissions)
	}
	manager.ProcessTimeoutRequests()
	if request = manager.matchQueues[1].TeamRequests[7]; request == nil || request.MatchID != "match-1" {
		t.Fatalf("timeout removed settling request: %#v", request)
	}

	manager.Matching()
	if len(rooms.admissions) != 2 || !reflect.DeepEqual(rooms.admissions[0], rooms.admissions[1]) {
		t.Fatalf("retried admissions = %#v", rooms.admissions)
	}
	if matchIDCalls != 1 || manager.matchQueues[1].IsTeamInQueue(7) || manager.matchQueues[1].IsPlayerInQueue(42) {
		t.Fatalf("resolved state = MatchID calls:%d requests:%#v players:%#v", matchIDCalls, manager.matchQueues[1].TeamRequests, manager.matchQueues[1].PlayerToTeam)
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
	queue.AddTeamRequest(&matchmodels.TeamMatchRequest{
		TeamId: 7, PlayerIds: []int64{42, 43}, TeamSize: 2, MatchType: 1,
	})
	queue.AddTeamRequest(&matchmodels.TeamMatchRequest{
		TeamId: 8, PlayerIds: []int64{44}, TeamSize: 1, MatchType: 1,
	})

	manager.Matching()

	if queue.IsTeamInQueue(7) || queue.IsPlayerInQueue(42) || queue.IsPlayerInQueue(43) {
		t.Fatalf("rejected Team remained queued: requests=%#v players=%#v", queue.TeamRequests, queue.PlayerToTeam)
	}
	request := queue.TeamRequests[8]
	if request == nil || request.MatchID != "" || !queue.IsPlayerInQueue(44) {
		t.Fatalf("non-rejected Team was not requeued: %#v", request)
	}
	if len(manager.settlements) != 0 || len(rooms.admissions) != 1 {
		t.Fatalf("settlement state = %#v, admissions=%d", manager.settlements, len(rooms.admissions))
	}
}

func useMatchManagerFactory(t *testing.T, factory matchManagerFactory) {
	t.Helper()
	previousRegistration := matchManagerRegistration
	previousFactory := registerMatchActor
	matchManagerRegistration = &matchManagerRegistry{}
	registerMatchActor = factory
	t.Cleanup(func() {
		matchManagerRegistration = previousRegistration
		registerMatchActor = previousFactory
	})
}

func matchPanicValue(f func()) (recovered any) {
	defer func() {
		recovered = recover()
	}()
	f()
	return nil
}

func TestGetMatchManagerBeforeRegistrationPanics(t *testing.T) {
	useMatchManagerFactory(t, func(playerread.PlayerReader, roomaccept.Acceptor, func() string, func() time.Time) *MatchManager {
		return &MatchManager{}
	})

	if got := matchPanicValue(func() { GetMatchManager() }); got != "match: GetMatchManager called before RegisterMatchManager" {
		t.Fatalf("注册前 GetMatchManager panic = %#v", got)
	}
}

func TestMatchManagerRegistrationWaitsUntilReady(t *testing.T) {
	registrationStarted := make(chan struct{})
	finishRegistration := make(chan struct{})
	want := &MatchManager{}
	useMatchManagerFactory(t, func(playerread.PlayerReader, roomaccept.Acceptor, func() string, func() time.Time) *MatchManager {
		close(registrationStarted)
		<-finishRegistration
		return want
	})
	rooms := &fakeRoomAcceptor{}
	newMatchID := func() string { return "match-1" }

	registered := make(chan *MatchManager, 1)
	go func() {
		registered <- RegisterMatchManager(&fakeMatchPlayerReader{}, rooms, newMatchID, time.Now)
	}()
	<-registrationStarted

	gotManager := make(chan *MatchManager, 1)
	go func() {
		gotManager <- GetMatchManager()
	}()
	select {
	case got := <-gotManager:
		t.Fatalf("注册完成前 GetMatchManager 返回了 %#v", got)
	case <-time.After(100 * time.Millisecond):
	}

	close(finishRegistration)
	select {
	case got := <-registered:
		if got != want {
			t.Fatalf("RegisterMatchManager 返回 %#v，期望 %#v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("RegisterMatchManager 未在注册完成后返回")
	}
	select {
	case got := <-gotManager:
		if got != want {
			t.Fatalf("GetMatchManager 返回 %#v，期望 %#v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("GetMatchManager 未在注册完成后唤醒")
	}
	if got := matchPanicValue(func() { RegisterMatchManager(&fakeMatchPlayerReader{}, rooms, newMatchID, time.Now) }); got != "match: RegisterMatchManager called more than once" {
		t.Fatalf("重复注册 panic = %#v", got)
	}
}

func TestMatchManagerRegistrationFailureIsTerminal(t *testing.T) {
	wantPanic := &struct{ reason string }{reason: "boom"}
	useMatchManagerFactory(t, func(playerread.PlayerReader, roomaccept.Acceptor, func() string, func() time.Time) *MatchManager {
		panic(wantPanic)
	})
	rooms := &fakeRoomAcceptor{}
	newMatchID := func() string { return "match-1" }

	if got := matchPanicValue(func() { RegisterMatchManager(&fakeMatchPlayerReader{}, rooms, newMatchID, time.Now) }); got != wantPanic {
		t.Fatalf("注册调用收到 panic %#v，期望 %#v", got, wantPanic)
	}
	if got := matchPanicValue(func() { GetMatchManager() }); got != wantPanic {
		t.Fatalf("失败后的 GetMatchManager panic = %#v，期望 %#v", got, wantPanic)
	}
	if got := matchPanicValue(func() { RegisterMatchManager(&fakeMatchPlayerReader{}, rooms, newMatchID, time.Now) }); got != "match: RegisterMatchManager called more than once" {
		t.Fatalf("失败后重试注册 panic = %#v", got)
	}
}

func TestRegisterMatchManagerNilReaderIsTerminal(t *testing.T) {
	factoryCalled := false
	useMatchManagerFactory(t, func(playerread.PlayerReader, roomaccept.Acceptor, func() string, func() time.Time) *MatchManager {
		factoryCalled = true
		return &MatchManager{}
	})

	wantPanic := "match: RegisterMatchManager requires PlayerReader"
	if got := matchPanicValue(func() { RegisterMatchManager(nil, &fakeRoomAcceptor{}, func() string { return "match-1" }, time.Now) }); got != wantPanic {
		t.Fatalf("nil PlayerReader 注册 panic = %#v，期望 %#v", got, wantPanic)
	}
	if factoryCalled {
		t.Fatal("nil PlayerReader 不应调用 MatchManager factory")
	}
	if got := matchPanicValue(func() { GetMatchManager() }); got != wantPanic {
		t.Fatalf("nil 注册失败后的 GetMatchManager panic = %#v，期望 %#v", got, wantPanic)
	}
}

func TestRegisterMatchManagerRejectsMissingRoomAcceptor(t *testing.T) {
	factoryCalled := false
	useMatchManagerFactory(t, func(playerread.PlayerReader, roomaccept.Acceptor, func() string, func() time.Time) *MatchManager {
		factoryCalled = true
		return &MatchManager{}
	})

	wantPanic := "match: RegisterMatchManager requires Room Acceptor"
	if got := matchPanicValue(func() {
		RegisterMatchManager(&fakeMatchPlayerReader{}, nil, func() string { return "match-1" }, time.Now)
	}); got != wantPanic {
		t.Fatalf("nil Room Acceptor panic = %#v，期望 %#v", got, wantPanic)
	}
	if factoryCalled {
		t.Fatal("nil Room Acceptor 不应调用 MatchManager factory")
	}
}

func TestRegisterMatchManagerRejectsMissingMatchIDGenerator(t *testing.T) {
	factoryCalled := false
	useMatchManagerFactory(t, func(playerread.PlayerReader, roomaccept.Acceptor, func() string, func() time.Time) *MatchManager {
		factoryCalled = true
		return &MatchManager{}
	})

	wantPanic := "match: RegisterMatchManager requires MatchID generator"
	if got := matchPanicValue(func() { RegisterMatchManager(&fakeMatchPlayerReader{}, &fakeRoomAcceptor{}, nil, time.Now) }); got != wantPanic {
		t.Fatalf("nil MatchID generator panic = %#v，期望 %#v", got, wantPanic)
	}
	if factoryCalled {
		t.Fatal("nil MatchID generator 不应调用 MatchManager factory")
	}
}

func TestRegisterMatchManagerRejectsMissingClock(t *testing.T) {
	factoryCalled := false
	useMatchManagerFactory(t, func(playerread.PlayerReader, roomaccept.Acceptor, func() string, func() time.Time) *MatchManager {
		factoryCalled = true
		return &MatchManager{}
	})

	wantPanic := "match: RegisterMatchManager requires Clock"
	if got := matchPanicValue(func() {
		RegisterMatchManager(&fakeMatchPlayerReader{}, &fakeRoomAcceptor{}, func() string { return "match-1" }, nil)
	}); got != wantPanic {
		t.Fatalf("nil Clock panic = %#v，期望 %#v", got, wantPanic)
	}
	if factoryCalled {
		t.Fatal("nil Clock 不应调用 MatchManager factory")
	}
}

func TestMatchManagerInitStoresRequiredDependencies(t *testing.T) {
	players := &fakeMatchPlayerReader{}
	rooms := &fakeRoomAcceptor{}
	newMatchID := func() string { return "match-1" }
	now := func() time.Time { return time.Unix(1, 0) }
	manager := &MatchManager{}

	manager.Init(players, rooms, newMatchID, now)

	if manager.players != players || manager.rooms != rooms || manager.newMatchID == nil || manager.now == nil || manager.matchQueues[1] == nil || manager.settlements == nil {
		t.Fatalf("initialized manager = %#v", manager)
	}
}
