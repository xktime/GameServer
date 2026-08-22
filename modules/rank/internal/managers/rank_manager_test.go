package managers

import (
	"gameserver/common/msg/message"
	rankmodels "gameserver/modules/rank/internal/models"
	"gameserver/modules/rank/playerread"
	"sync"
	"testing"
	"time"
)

type fakePlayerReader map[int64]playerread.PlayerSnapshot

func (f fakePlayerReader) FindOnline(playerID int64) (playerread.PlayerSnapshot, bool) {
	snapshot, ok := f[playerID]
	return snapshot, ok
}

func TestRankManagerRejectsOfflinePlayerUpdate(t *testing.T) {
	manager := &RankManager{players: fakePlayerReader{}}

	response := manager.doHandleUpdateRankData(42, &message.C2S_UpdateRankData{
		RankType: message.RankType_RankType_ChallengePoint,
		Score:    100,
	})

	if response.Success {
		t.Fatal("离线 Player 不应更新排行榜")
	}
}

func TestRankManagerUpdatesRankFromOnlinePlayerSnapshot(t *testing.T) {
	seasonManager = &SeasonManager{Season: 1}
	seasonManagerOnce = sync.Once{}
	seasonManagerOnce.Do(func() {})
	t.Cleanup(func() {
		seasonManager = nil
		seasonManagerOnce = sync.Once{}
	})

	rankType := message.RankType_RankType_ChallengePoint
	rankData := &rankmodels.RankData{
		RankType:   rankType,
		Items:      make([]*rankmodels.RankItem, 0),
		ItemsCache: make(map[int64]*rankmodels.RankItem),
	}
	manager := &RankManager{
		players: fakePlayerReader{
			42: {
				Name:      "测试玩家",
				AvatarURL: "https://example.com/avatar.png",
				Level:     12,
			},
		},
		RankCache: map[message.RankType]map[int32]*rankmodels.RankData{
			rankType: {0: rankData},
		},
	}

	response := manager.doHandleUpdateRankData(42, &message.C2S_UpdateRankData{
		RankType: rankType,
		Score:    100,
	})

	if !response.Success {
		t.Fatal("在线 Player 应成功更新排行榜")
	}
	if len(rankData.Items) != 1 {
		t.Fatalf("排行榜条目数量 = %d，期望 1", len(rankData.Items))
	}
	item := rankData.Items[0]
	if item.PlayerName != "测试玩家" || item.Avatar != "https://example.com/avatar.png" || item.Level != 12 {
		t.Fatalf("排行榜条目未使用 Player 快照: %#v", item)
	}
}

func TestRankManagerRejectsOfflinePlayerRankList(t *testing.T) {
	manager := &RankManager{players: fakePlayerReader{}}

	response := manager.doHandleGetRankList(42, &message.C2S_GetRankList{
		RankType: message.RankType_RankType_ChallengePoint,
		Page:     1,
		PageSize: 20,
	})

	if response != nil {
		t.Fatalf("离线 Player 的排行榜列表响应 = %#v，期望 nil", response)
	}
}

func TestRankManagerReturnsEmptyRankForOfflinePlayer(t *testing.T) {
	manager := &RankManager{players: fakePlayerReader{}}
	rankType := message.RankType_RankType_ChallengePoint

	response := manager.doHandleGetMyRank(42, rankType, 0)

	if response == nil {
		t.Fatal("离线 Player 应收到空排名响应")
	}
	if response.RankType != rankType || response.MyRank != 0 || response.MyScore != 0 || response.TotalCount != 0 {
		t.Fatalf("离线 Player 的空排名响应不符合预期: %#v", response)
	}
}

func useRankManagerFactory(t *testing.T, factory rankManagerFactory) {
	t.Helper()
	previousRegistration := rankManagerRegistration
	previousFactory := registerRankActor
	rankManagerRegistration = &rankManagerRegistry{}
	registerRankActor = factory
	t.Cleanup(func() {
		rankManagerRegistration = previousRegistration
		registerRankActor = previousFactory
	})
}

func panicValue(f func()) (recovered any) {
	defer func() {
		recovered = recover()
	}()
	f()
	return nil
}

func TestGetRankManagerBeforeRegistrationPanics(t *testing.T) {
	useRankManagerFactory(t, func(playerread.PlayerReader) *RankManager {
		return &RankManager{}
	})

	got := panicValue(func() {
		GetRankManager()
	})
	if got != "rank: GetRankManager called before RegisterRankManager" {
		t.Fatalf("注册前 GetRankManager panic = %#v", got)
	}
}

func TestRankManagerRegistrationWaitsUntilReadyAndRejectsDuplicates(t *testing.T) {
	registrationStarted := make(chan struct{})
	finishRegistration := make(chan struct{})
	want := &RankManager{}
	useRankManagerFactory(t, func(playerread.PlayerReader) *RankManager {
		close(registrationStarted)
		<-finishRegistration
		return want
	})

	registered := make(chan *RankManager, 1)
	go func() {
		registered <- RegisterRankManager(fakePlayerReader{})
	}()
	<-registrationStarted

	duplicatePanic := panicValue(func() {
		RegisterRankManager(fakePlayerReader{})
	})
	if duplicatePanic != "rank: RegisterRankManager called more than once" {
		t.Fatalf("注册进行中重复注册 panic = %#v", duplicatePanic)
	}

	gotManager := make(chan *RankManager, 1)
	go func() {
		gotManager <- GetRankManager()
	}()
	select {
	case got := <-gotManager:
		t.Fatalf("注册完成前 GetRankManager 返回了 %#v", got)
	case <-time.After(100 * time.Millisecond):
	}

	close(finishRegistration)
	select {
	case got := <-registered:
		if got != want {
			t.Fatalf("RegisterRankManager 返回 %#v，期望 %#v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("RegisterRankManager 未在注册完成后返回")
	}
	select {
	case got := <-gotManager:
		if got != want {
			t.Fatalf("GetRankManager 返回 %#v，期望 %#v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("GetRankManager 未在注册完成后唤醒")
	}
	if got := GetRankManager(); got != want {
		t.Fatalf("注册完成后 GetRankManager 返回 %#v，期望 %#v", got, want)
	}
}

func TestRankManagerRegistrationFailureIsTerminal(t *testing.T) {
	registrationStarted := make(chan struct{})
	failRegistration := make(chan struct{})
	wantPanic := &struct{ reason string }{reason: "boom"}
	useRankManagerFactory(t, func(playerread.PlayerReader) *RankManager {
		close(registrationStarted)
		<-failRegistration
		panic(wantPanic)
	})

	registrationPanic := make(chan any, 1)
	go func() {
		registrationPanic <- panicValue(func() {
			RegisterRankManager(fakePlayerReader{})
		})
	}()
	<-registrationStarted

	waiterPanic := make(chan any, 1)
	go func() {
		waiterPanic <- panicValue(func() {
			GetRankManager()
		})
	}()
	select {
	case got := <-waiterPanic:
		t.Fatalf("注册失败前等待者提前返回 panic %#v", got)
	case <-time.After(100 * time.Millisecond):
	}

	close(failRegistration)
	select {
	case got := <-registrationPanic:
		if got != wantPanic {
			t.Fatalf("注册调用收到 panic %#v，期望 %#v", got, wantPanic)
		}
	case <-time.After(time.Second):
		t.Fatal("注册失败未传播给注册调用")
	}
	select {
	case got := <-waiterPanic:
		if got != wantPanic {
			t.Fatalf("等待者收到 panic %#v，期望 %#v", got, wantPanic)
		}
	case <-time.After(time.Second):
		t.Fatal("注册失败未唤醒等待者")
	}
	if got := panicValue(func() { GetRankManager() }); got != wantPanic {
		t.Fatalf("失败后的 GetRankManager panic = %#v，期望 %#v", got, wantPanic)
	}
	if got := panicValue(func() { RegisterRankManager(fakePlayerReader{}) }); got != "rank: RegisterRankManager called more than once" {
		t.Fatalf("失败后重试注册 panic = %#v", got)
	}
}

func TestRegisterRankManagerNilReaderIsTerminal(t *testing.T) {
	factoryCalled := false
	useRankManagerFactory(t, func(playerread.PlayerReader) *RankManager {
		factoryCalled = true
		return &RankManager{}
	})

	wantPanic := "rank: RegisterRankManager requires PlayerReader"
	if got := panicValue(func() { RegisterRankManager(nil) }); got != wantPanic {
		t.Fatalf("nil PlayerReader 注册 panic = %#v，期望 %#v", got, wantPanic)
	}
	if factoryCalled {
		t.Fatal("nil PlayerReader 不应调用 RankManager factory")
	}
	if got := panicValue(func() { GetRankManager() }); got != wantPanic {
		t.Fatalf("nil 注册失败后的 GetRankManager panic = %#v，期望 %#v", got, wantPanic)
	}
	if got := panicValue(func() { RegisterRankManager(fakePlayerReader{}) }); got != "rank: RegisterRankManager called more than once" {
		t.Fatalf("nil 注册失败后重试 panic = %#v", got)
	}
}
