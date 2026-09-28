package managers

import (
	"errors"
	"gameserver/common/msg/message"
	rankmodels "gameserver/modules/rank/internal/models"
	"gameserver/modules/rank/playerread"
	"testing"
	"time"
)

type fakePlayerReader map[int64]playerread.PlayerSnapshot

func (f fakePlayerReader) FindOnline(playerID int64) (playerread.PlayerSnapshot, bool) {
	snapshot, ok := f[playerID]
	return snapshot, ok
}

func rankTestNow() time.Time {
	return time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
}

func TestRankManagerInitializationCommandCanRetry(t *testing.T) {
	repository := &memoryRankRepository{loadSeasonErrors: []error{errors.New("database unavailable")}}
	if err := repository.SaveSeason(&SeasonManager{PersistId: 1, Season: 3, LastCrossDayTime: rankTestNow().Unix()}); err != nil {
		t.Fatal(err)
	}
	manager, _ := startTestRankManager(t, repository, fakePlayerReader{}, rankTestNow)
	if season := manager.GetSeasonInfo(); season == nil || season.Season != 3 {
		t.Fatalf("重试后未恢复赛季: %v", season)
	}
}

func TestRankManagerInitializesMissingPersistedCache(t *testing.T) {
	manager, _ := startTestRankManager(t, &memoryRankRepository{}, fakePlayerReader{42: {}}, rankTestNow)
	for _, rankType := range types {
		response := manager.HandleUpdateRankData(42, &message.C2S_UpdateRankData{RankType: rankType, Score: 10})
		if !response.Success {
			t.Fatalf("空数据库的排行榜类型 %v 无法更新", rankType)
		}
	}
}

func TestRankManagerRebuildsPersistedRankIndexBeforeUpdatingPlayer(t *testing.T) {
	rankType := message.RankType_RankType_ChallengePoint
	repository := &memoryRankRepository{}
	if err := repository.SaveRank(&RankManager{
		PersistId: 1,
		RankCache: map[message.RankType]map[int32]*rankmodels.RankData{
			rankType: {0: {RankType: rankType, Items: []*rankmodels.RankItem{
				{PlayerId: 42, Score: 10, Rank: 1, UpdateTime: rankTestNow()},
			}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	manager, _ := startTestRankManager(t, repository, fakePlayerReader{42: {Name: "测试玩家"}}, rankTestNow)
	if response := manager.HandleUpdateRankData(42, &message.C2S_UpdateRankData{RankType: rankType, Score: 20}); !response.Success {
		t.Fatal("恢复后的排行榜应允许更新已有 Player")
	}
	response := manager.HandleGetMyRank(42, rankType, 0)
	if response == nil || response.MyScore != 20 || response.TotalCount != 1 {
		t.Fatalf("恢复索引后重复添加了 Player 或未更新分数: %v", response)
	}
}

func TestRankManagerReturnsEmptyChallengeListForEmptyRank(t *testing.T) {
	manager, _ := startTestRankManager(t, &memoryRankRepository{}, fakePlayerReader{42: {}}, rankTestNow)
	response := manager.GetChallengeList(42, &message.C2S_GetChanllengeList{RankType: message.RankType_RankType_ChallengePoint})
	if response == nil || response.RankItem == nil || len(response.RankItem) != 0 {
		t.Fatalf("空排行榜应返回空挑战列表: %v", response)
	}
}

func TestRankManagerReturnsEmptyRankListForUnknownRankType(t *testing.T) {
	manager, _ := startTestRankManager(t, &memoryRankRepository{}, fakePlayerReader{42: {}}, rankTestNow)
	unknownRankType := message.RankType(99)
	response := manager.HandleGetRankList(42, &message.C2S_GetRankList{RankType: unknownRankType, Page: 1, PageSize: 20})
	if response == nil || response.RankType != unknownRankType || response.TotalCount != 0 || len(response.RankItems) != 0 {
		t.Fatalf("未知 RankType 的响应不符合预期: %v", response)
	}
}

func TestRankManagerReturnsEmptyMyRankForUnknownRankType(t *testing.T) {
	manager, system := startTestRankManager(t, &memoryRankRepository{}, fakePlayerReader{42: {}}, rankTestNow)
	unknownRankType := message.RankType(99)
	response := manager.HandleGetMyRank(42, unknownRankType, 0)
	if response == nil || response.RankType != unknownRankType || response.MyRank != 0 || response.MyScore != 0 || response.TotalCount != 0 {
		t.Fatalf("未知 RankType 的玩家排名响应不符合预期: %v", response)
	}
	if panics := system.Metrics().HandlerPanics; panics != 0 {
		t.Fatalf("未知 RankType 导致 Actor panic 次数 = %d，期望 0", panics)
	}
}

func TestRankManagerRejectsUnknownRankTypeUpdateWithoutActorPanic(t *testing.T) {
	manager, system := startTestRankManager(t, &memoryRankRepository{}, fakePlayerReader{42: {}}, rankTestNow)
	response := manager.HandleUpdateRankData(42, &message.C2S_UpdateRankData{RankType: message.RankType(99), Score: 100})
	if response == nil || response.Success {
		t.Fatalf("未知 RankType 的更新响应 = %v，期望失败", response)
	}
	if panics := system.Metrics().HandlerPanics; panics != 0 {
		t.Fatalf("未知 RankType 更新导致 Actor panic 次数 = %d，期望 0", panics)
	}
}

func TestRankManagerRejectsOfflinePlayerUpdate(t *testing.T) {
	manager, _ := startTestRankManager(t, &memoryRankRepository{}, fakePlayerReader{}, rankTestNow)
	response := manager.HandleUpdateRankData(42, &message.C2S_UpdateRankData{RankType: message.RankType_RankType_ChallengePoint, Score: 100})
	if response.Success {
		t.Fatal("离线 Player 不应更新排行榜")
	}
}

func TestRankManagerUpdatesRankFromOnlinePlayerSnapshot(t *testing.T) {
	rankType := message.RankType_RankType_ChallengePoint
	manager, _ := startTestRankManager(t, &memoryRankRepository{}, fakePlayerReader{
		42: {Name: "测试玩家", AvatarURL: "https://example.com/avatar.png", Level: 12},
	}, rankTestNow)
	if response := manager.HandleUpdateRankData(42, &message.C2S_UpdateRankData{RankType: rankType, Score: 100}); !response.Success {
		t.Fatal("在线 Player 应成功更新排行榜")
	}
	response := manager.HandleGetRankList(42, &message.C2S_GetRankList{RankType: rankType, Page: 1, PageSize: 20})
	if response == nil || len(response.RankItems) != 1 {
		t.Fatalf("排行榜条目不符合预期: %v", response)
	}
	item := response.RankItems[0]
	if item.PlayerName != "测试玩家" || item.Avatar != "https://example.com/avatar.png" || item.Level != 12 {
		t.Fatalf("排行榜条目未使用 Player 快照: %v", item)
	}
}

func TestRankManagerRejectsOfflinePlayerRankList(t *testing.T) {
	manager, _ := startTestRankManager(t, &memoryRankRepository{}, fakePlayerReader{}, rankTestNow)
	response := manager.HandleGetRankList(42, &message.C2S_GetRankList{RankType: message.RankType_RankType_ChallengePoint, Page: 1, PageSize: 20})
	if response != nil {
		t.Fatalf("离线 Player 的排行榜列表响应 = %v，期望 nil", response)
	}
}

func TestRankManagerReturnsEmptyRankForOfflinePlayer(t *testing.T) {
	manager, _ := startTestRankManager(t, &memoryRankRepository{}, fakePlayerReader{}, rankTestNow)
	rankType := message.RankType_RankType_ChallengePoint
	response := manager.HandleGetMyRank(42, rankType, 0)
	if response == nil || response.RankType != rankType || response.MyRank != 0 || response.MyScore != 0 || response.TotalCount != 0 {
		t.Fatalf("离线 Player 的空排名响应不符合预期: %v", response)
	}
}

func TestRankManagerUsesClockToOrderEqualScores(t *testing.T) {
	// 已有记录在真实时间的未来，只有注入时钟才能把新记录正确排在其后。
	now := time.Now().UTC().Add(24 * time.Hour)
	rankType := message.RankType_RankType_ChallengePoint
	repository := &memoryRankRepository{}
	if err := repository.SaveRank(&RankManager{
		PersistId: 1,
		RankCache: map[message.RankType]map[int32]*rankmodels.RankData{
			rankType: {0: {RankType: rankType, Items: []*rankmodels.RankItem{
				{PlayerId: 43, Score: 10, UpdateTime: now.Add(-time.Hour)},
			}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	manager, _ := startTestRankManager(t, repository, fakePlayerReader{42: {}}, func() time.Time { return now })
	if response := manager.HandleUpdateRankData(42, &message.C2S_UpdateRankData{RankType: rankType, Score: 10}); !response.Success {
		t.Fatal("更新排名失败")
	}
	response := manager.HandleGetMyRank(42, rankType, 0)
	if response == nil || response.MyRank != 2 {
		t.Fatalf("同分玩家应按注入时钟的更新时间排序: %v", response)
	}
}
