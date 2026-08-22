package managers

import (
	commonmodels "gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	"gameserver/core/gate"
	matchmodels "gameserver/modules/match/internal/models"
	"gameserver/modules/match/playerread"
	"net"
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
	players    map[int64]playerread.PlayerSnapshot
	teams      map[int64]playerread.TeamSnapshot
	randomIDs  []int64
	exclusions [][]int64
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

func (f *fakeMatchPlayerReader) FindRandomOnline(excludedPlayerIDs []int64) (int64, bool) {
	f.exclusions = append(f.exclusions, append([]int64(nil), excludedPlayerIDs...))
	if len(f.randomIDs) == 0 {
		return 0, false
	}
	playerID := f.randomIDs[0]
	f.randomIDs = f.randomIDs[1:]
	return playerID, true
}

func newMatchTestManager(players playerread.PlayerReader) *MatchManager {
	return &MatchManager{
		players: players,
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

func TestMatchManagerBuildsRobotTeamsFromOnlinePlayers(t *testing.T) {
	utils.InitSnowflake(1)
	players := &fakeMatchPlayerReader{randomIDs: []int64{100, 101}}
	manager := newMatchTestManager(players)

	teams := manager.randomRobotTeams(1, 2, []int64{42})
	if len(teams) != 2 {
		t.Fatalf("机器人 Team 数量 = %d，期望 2", len(teams))
	}
	if !slices.Equal(teams[0].PlayerIds, []int64{100}) || !slices.Equal(teams[1].PlayerIds, []int64{101}) {
		t.Fatalf("机器人 Player = %#v", teams)
	}
	if !teams[0].IsRobot || !teams[1].IsRobot || teams[0].TeamId == teams[1].TeamId {
		t.Fatalf("机器人 Team 属性不正确: %#v", teams)
	}
	if len(players.exclusions) != 2 || !slices.Equal(players.exclusions[0], []int64{42}) || !slices.Equal(players.exclusions[1], []int64{42, 100}) {
		t.Fatalf("随机候选排除列表 = %#v", players.exclusions)
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
	useMatchManagerFactory(t, func(playerread.PlayerReader) *MatchManager {
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
	useMatchManagerFactory(t, func(playerread.PlayerReader) *MatchManager {
		close(registrationStarted)
		<-finishRegistration
		return want
	})

	registered := make(chan *MatchManager, 1)
	go func() {
		registered <- RegisterMatchManager(&fakeMatchPlayerReader{})
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
	if got := matchPanicValue(func() { RegisterMatchManager(&fakeMatchPlayerReader{}) }); got != "match: RegisterMatchManager called more than once" {
		t.Fatalf("重复注册 panic = %#v", got)
	}
}

func TestMatchManagerRegistrationFailureIsTerminal(t *testing.T) {
	wantPanic := &struct{ reason string }{reason: "boom"}
	useMatchManagerFactory(t, func(playerread.PlayerReader) *MatchManager {
		panic(wantPanic)
	})

	if got := matchPanicValue(func() { RegisterMatchManager(&fakeMatchPlayerReader{}) }); got != wantPanic {
		t.Fatalf("注册调用收到 panic %#v，期望 %#v", got, wantPanic)
	}
	if got := matchPanicValue(func() { GetMatchManager() }); got != wantPanic {
		t.Fatalf("失败后的 GetMatchManager panic = %#v，期望 %#v", got, wantPanic)
	}
	if got := matchPanicValue(func() { RegisterMatchManager(&fakeMatchPlayerReader{}) }); got != "match: RegisterMatchManager called more than once" {
		t.Fatalf("失败后重试注册 panic = %#v", got)
	}
}

func TestRegisterMatchManagerNilReaderIsTerminal(t *testing.T) {
	factoryCalled := false
	useMatchManagerFactory(t, func(playerread.PlayerReader) *MatchManager {
		factoryCalled = true
		return &MatchManager{}
	})

	wantPanic := "match: RegisterMatchManager requires PlayerReader"
	if got := matchPanicValue(func() { RegisterMatchManager(nil) }); got != wantPanic {
		t.Fatalf("nil PlayerReader 注册 panic = %#v，期望 %#v", got, wantPanic)
	}
	if factoryCalled {
		t.Fatal("nil PlayerReader 不应调用 MatchManager factory")
	}
	if got := matchPanicValue(func() { GetMatchManager() }); got != wantPanic {
		t.Fatalf("nil 注册失败后的 GetMatchManager panic = %#v，期望 %#v", got, wantPanic)
	}
}
