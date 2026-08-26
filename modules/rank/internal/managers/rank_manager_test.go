package managers

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
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

func TestRankManagerInitializationCommandCanRetry(t *testing.T) {
	system := actor.NewActorSystem(time.Second)
	t.Cleanup(func() {
		if err := system.Stop(context.Background()); err != nil {
			t.Errorf("停止 ActorSystem: %v", err)
		}
	})
	scope, err := system.NewScope("rank-manager-initialization-test")
	if err != nil {
		t.Fatalf("创建 Scope: %v", err)
	}

	loadFailure := errors.New("database unavailable")
	attempts := 0
	manager, err := newRankManager(
		context.Background(),
		scope,
		fakePlayerReader{},
		func() (*SeasonManager, error) {
			attempts++
			if attempts == 1 {
				return nil, loadFailure
			}
			return &SeasonManager{PersistId: 1, Season: 3}, nil
		},
		func(*RankManager) error { return nil },
		func() time.Time { return time.Unix(100, 0) },
	)
	if err != nil {
		t.Fatalf("初始化 RankManager: %v", err)
	}

	if !manager.initialized || manager.season.Season != 3 || attempts != 2 {
		t.Fatalf("重试结果不正确: initialized=%t season=%d attempts=%d", manager.initialized, manager.season.Season, attempts)
	}
}

func TestRankManagerInitializesMissingPersistedCache(t *testing.T) {
	manager := &RankManager{season: &SeasonManager{Season: 1}}

	manager.initializeRankTypes()

	for _, rankType := range types {
		if manager.RankCache[rankType] == nil {
			t.Fatalf("排行榜类型 %v 未初始化", rankType)
		}
	}
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
	rankType := message.RankType_RankType_ChallengePoint
	rankData := &rankmodels.RankData{
		RankType:   rankType,
		Items:      make([]*rankmodels.RankItem, 0),
		ItemsCache: make(map[int64]*rankmodels.RankItem),
	}
	manager := &RankManager{
		season: &SeasonManager{Season: 1},
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
