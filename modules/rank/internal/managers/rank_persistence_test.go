package managers

import (
	"context"
	"errors"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/db/mongodb"
	"gameserver/common/msg/message"
	"gameserver/modules/rank/internal/documents"
	rankmodels "gameserver/modules/rank/internal/models"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// 内存适配器按文档保存 BSON；与 Mongo 共用 rankRepository 的迁移及保存流程。
type memoryRankStore struct {
	rank              []byte
	season            []byte
	boards            map[string][]byte
	loadSeasonErrors  []error
	loadRankError     error
	loadBoardsError   error
	saveRankError     error
	saveSeasonError   error
	saveBoardErrors   map[string]error
	insertBoardErrors map[string]error
	deleteError       error
}

func (r *memoryRankStore) LoadRank() (*documents.RankManager, error) {
	if r.loadRankError != nil {
		return nil, r.loadRankError
	}
	if len(r.rank) == 0 {
		return nil, nil
	}
	var data documents.RankManager
	err := bson.Unmarshal(r.rank, &data)
	return &data, err
}

func (r *memoryRankStore) LoadSeason() (*SeasonManager, error) {
	if len(r.loadSeasonErrors) > 0 {
		err := r.loadSeasonErrors[0]
		r.loadSeasonErrors = r.loadSeasonErrors[1:]
		return nil, err
	}
	if len(r.season) == 0 {
		return nil, nil
	}
	var data SeasonManager
	err := bson.Unmarshal(r.season, &data)
	return &data, err
}

func (r *memoryRankStore) LoadBoards() ([]documents.Leaderboard, error) {
	if r.loadBoardsError != nil {
		return nil, r.loadBoardsError
	}
	keys := make([]string, 0, len(r.boards))
	for key := range r.boards {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]documents.Leaderboard, 0, len(keys))
	for _, key := range keys {
		var board documents.Leaderboard
		if err := bson.Unmarshal(r.boards[key], &board); err != nil {
			return nil, err
		}
		result = append(result, board)
	}
	return result, nil
}

func (r *memoryRankStore) Save(data mongodb.PersistData) error {
	switch doc := data.(type) {
	case *documents.RankManager:
		if r.saveRankError != nil {
			return r.saveRankError
		}
		bytes, err := bson.Marshal(doc)
		if err == nil {
			r.rank = bytes
		}
		return err
	case *SeasonManager:
		if r.saveSeasonError != nil {
			return r.saveSeasonError
		}
		bytes, err := bson.Marshal(doc)
		if err == nil {
			r.season = bytes
		}
		return err
	case *documents.Leaderboard:
		if err := r.saveBoardErrors[doc.PersistId]; err != nil {
			return err
		}
		bytes, err := bson.Marshal(doc)
		if err == nil {
			if r.boards == nil {
				r.boards = make(map[string][]byte)
			}
			r.boards[doc.PersistId] = bytes
		}
		return err
	default:
		return fmt.Errorf("unsupported rank document: %T", data)
	}
}

func (r *memoryRankStore) InsertBoard(board *documents.Leaderboard) error {
	if err := r.insertBoardErrors[board.PersistId]; err != nil {
		return err
	}
	if _, exists := r.boards[board.PersistId]; exists {
		return nil
	}
	return r.Save(board)
}

func (r *memoryRankStore) DeleteBoard(id string) error {
	if r.deleteError != nil {
		return r.deleteError
	}
	delete(r.boards, id)
	return nil
}

func startTestRankManager(t *testing.T, repository rankDocumentStore, players fakePlayerReader, now func() time.Time) (*RankManager, *actor.ActorSystem) {
	t.Helper()
	system := actor.NewActorSystem(time.Second)
	t.Cleanup(func() {
		if err := system.Stop(context.Background()); err != nil {
			t.Errorf("停止 Rank ActorSystem: %v", err)
		}
	})
	scope, err := system.NewScope("rank-test")
	if err != nil {
		t.Fatalf("创建 Rank Scope: %v", err)
	}
	initialization, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	manager, err := newRankManager(initialization, scope, players, repository, now)
	if err != nil {
		t.Fatalf("创建 RankManager: %v", err)
	}
	return manager, system
}

func TestRankManagerPersistsRankAcrossActorRestart(t *testing.T) {
	repository := &memoryRankStore{}
	now := func() time.Time { return time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC) }
	players := fakePlayerReader{42: {Name: "测试玩家", AvatarURL: "avatar", Level: 12}}
	manager, system := startTestRankManager(t, repository, players, now)
	rankType := message.RankType_RankType_ChallengePoint
	if response := manager.HandleUpdateRankData(42, &message.C2S_UpdateRankData{RankType: rankType, Score: 123}); !response.Success {
		t.Fatal("更新排行榜失败")
	}
	if err := system.Stop(context.Background()); err != nil {
		t.Fatalf("保存并停止 Rank: %v", err)
	}

	restarted, _ := startTestRankManager(t, repository, players, now)
	response := restarted.HandleGetMyRank(42, rankType, 0)
	if response == nil || response.MyScore != 123 || response.MyRank != 1 || response.TotalCount != 1 {
		t.Fatalf("重启后未恢复排名: %v", response)
	}
}

func TestRankManagerRestoresChallengeCodesAndGenerationProgress(t *testing.T) {
	repository := &memoryRankStore{}
	players := fakePlayerReader{42: {Name: "挑战目标"}}
	manager, system := startTestRankManager(t, repository, players, rankTestNow)
	code := manager.GeneratorChallengeCode(&message.C2S_GeneratorChanllengeCode{PlayerId: "42"})
	if code == nil {
		t.Fatal("生成挑战码失败")
	}
	if err := system.Stop(context.Background()); err != nil {
		t.Fatalf("保存挑战码: %v", err)
	}

	restarted, _ := startTestRankManager(t, repository, players, rankTestNow)
	response := restarted.ChanllengeByCode(&message.C2S_ChanllengeByCode{Code: code.Code})
	if response == nil || response.Result != message.ChanllengeResult_ChanllengeResult_SUCCESS || response.RankItem == nil || response.RankItem.PlayerId != "42" {
		t.Fatalf("正常重启后未过期挑战码失效: %v", response)
	}
	next := restarted.GeneratorChallengeCode(&message.C2S_GeneratorChanllengeCode{PlayerId: "42"})
	if next == nil || next.Code != code.Code+1 {
		t.Fatalf("重启后未延续挑战码生成进度: previous=%v next=%v", code, next)
	}
}

func TestRankManagerKeepsChallengeCodeExpiryAcrossRestart(t *testing.T) {
	var clock atomic.Int64
	clock.Store(rankTestNow().Unix())
	now := func() time.Time { return time.Unix(clock.Load(), 0) }
	repository := &memoryRankStore{}
	players := fakePlayerReader{42: {Name: "挑战目标"}}
	manager, system := startTestRankManager(t, repository, players, now)
	code := manager.GeneratorChallengeCode(&message.C2S_GeneratorChanllengeCode{PlayerId: "42"})
	expiresAt := rankTestNow().Unix() + int64(rankmodels.ChallengeCodeExpireTime)
	if code == nil || int64(code.ExpireTimestamp) != expiresAt {
		t.Fatalf("挑战码未使用注入时钟: response=%v expectedExpiry=%d", code, expiresAt)
	}
	if err := system.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}

	clock.Store(expiresAt)
	restarted, _ := startTestRankManager(t, repository, players, now)
	response := restarted.ChanllengeByCode(&message.C2S_ChanllengeByCode{Code: code.Code})
	if response == nil || response.Result != message.ChanllengeResult_ChanllengeResult_SUCCESS {
		t.Fatalf("到期边界应保留原有的小于判断语义: %v", response)
	}
	clock.Store(expiresAt + 1)
	response = restarted.ChanllengeByCode(&message.C2S_ChanllengeByCode{Code: code.Code})
	if response == nil || response.Result != message.ChanllengeResult_ChanllengeResult_CodeExpired {
		t.Fatalf("重启不应延长挑战码有效期: %v", response)
	}
}

func TestRankManagerUsesClockForCrossDayAndRestoresSeason(t *testing.T) {
	var clock atomic.Int64
	clock.Store(rankTestNow().Unix())
	now := func() time.Time { return time.Unix(clock.Load(), 0) }
	repository := &memoryRankStore{}
	players := fakePlayerReader{42: {Name: "测试玩家"}}
	manager, system := startTestRankManager(t, repository, players, now)
	if response := manager.HandleUpdateRankData(42, &message.C2S_UpdateRankData{RankType: message.RankType_RankType_LadderPoint, Score: 123}); !response.Success {
		t.Fatal("更新天梯排行榜失败")
	}
	initialSeason := manager.GetSeasonInfo()
	if initialSeason == nil {
		t.Fatal("未初始化赛季")
	}

	clock.Add(int64(24 * time.Hour / time.Second))
	manager.CheckCrossDay()
	history := manager.GetMyHistoryRank(42)
	if history == nil || history.Daily == nil || history.Daily.LastRank != 1 {
		t.Fatalf("注入时钟跨天后应保存昨日排名: %v", history)
	}
	clock.Store(rankTestNow().AddDate(0, 0, 7).Unix())
	manager.CheckCrossDay()
	season := manager.GetSeasonInfo()
	if season == nil || season.Season != initialSeason.Season+1 {
		t.Fatalf("跨周应推进赛季: before=%v after=%v", initialSeason, season)
	}
	if err := system.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}

	restarted, _ := startTestRankManager(t, repository, players, now)
	restoredSeason := restarted.GetSeasonInfo()
	if restoredSeason == nil || restoredSeason.Season != season.Season || restoredSeason.SeasonStartTimestamp != season.SeasonStartTimestamp || restoredSeason.SeasonEndTimestamp != season.SeasonEndTimestamp {
		t.Fatalf("重启后赛季不一致: expected=%v actual=%v", season, restoredSeason)
	}
	history = restarted.GetMyHistoryRank(42)
	if history == nil || history.Week == nil || history.Week.LastRank != 1 {
		t.Fatalf("重启后未恢复上赛季排名: %v", history)
	}
}

func TestRankManagerRetriesFailedPersistenceWhenStopping(t *testing.T) {
	rankFailure := errors.New("rank save failed")
	seasonFailure := errors.New("season save failed")
	for _, test := range []struct {
		name      string
		rankErr   error
		seasonErr error
	}{
		{name: "rank", rankErr: rankFailure},
		{name: "season", seasonErr: seasonFailure},
		{name: "both", rankErr: rankFailure, seasonErr: seasonFailure},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &memoryRankStore{}
			players := fakePlayerReader{42: {Name: "测试玩家"}}
			manager, system := startTestRankManager(t, repository, players, rankTestNow)
			rankType := message.RankType_RankType_ChallengePoint
			if response := manager.HandleUpdateRankData(42, &message.C2S_UpdateRankData{RankType: rankType, Score: 123}); !response.Success {
				t.Fatal("更新排行榜失败")
			}
			repository.saveRankError = test.rankErr
			repository.saveSeasonError = test.seasonErr
			defer func() {
				repository.saveRankError = nil
				repository.saveSeasonError = nil
			}()
			err := system.Stop(context.Background())
			if err == nil || (test.rankErr != nil && !errors.Is(err, test.rankErr)) || (test.rankErr == nil && test.seasonErr != nil && !errors.Is(err, test.seasonErr)) {
				t.Fatalf("停止未返回失败阶段的保存错误: %v", err)
			}
			if response := manager.HandleUpdateRankData(42, &message.C2S_UpdateRankData{RankType: rankType, Score: 999}); response.Success {
				t.Fatal("保存失败的 Actor 不应继续接受修改")
			}

			repository.saveRankError = nil
			repository.saveSeasonError = nil
			if err := system.Stop(context.Background()); err != nil {
				t.Fatalf("重试保存失败: %v", err)
			}
			restarted, _ := startTestRankManager(t, repository, players, rankTestNow)
			response := restarted.HandleGetMyRank(42, rankType, 0)
			if response == nil || response.MyScore != 123 {
				t.Fatalf("重试未保存停止前的状态: %v", response)
			}
		})
	}
}

func TestRankManagerLoadsLegacyDocumentsWithoutChallengeFields(t *testing.T) {
	for _, cache := range []struct {
		name string
		data bson.M
	}{
		{name: "missing", data: bson.M{"_id": int64(1)}},
		{name: "null", data: bson.M{"_id": int64(1), "rank_cache": nil, "challenge_code_cache": nil}},
	} {
		t.Run(cache.name, func(t *testing.T) {
			document, err := bson.Marshal(cache.data)
			if err != nil {
				t.Fatal(err)
			}
			repository := &memoryRankStore{rank: document}
			manager, _ := startTestRankManager(t, repository, fakePlayerReader{42: {}}, rankTestNow)
			code := manager.GeneratorChallengeCode(&message.C2S_GeneratorChanllengeCode{PlayerId: "42"})
			if code == nil || code.Code != rankmodels.ChallengeCodeFloorint+1 {
				t.Fatalf("旧文档未补齐挑战码默认状态: %v", code)
			}
			response := manager.ChanllengeByCode(&message.C2S_ChanllengeByCode{Code: code.Code})
			if response == nil || response.Result != message.ChanllengeResult_ChanllengeResult_SUCCESS {
				t.Fatalf("旧文档初始化后无法使用挑战码: %v", response)
			}
		})
	}
}
