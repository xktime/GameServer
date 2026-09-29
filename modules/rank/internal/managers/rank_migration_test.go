package managers

import (
	"bytes"
	"context"
	"errors"
	"gameserver/common/db/mongodb"
	"gameserver/common/msg/message"
	"gameserver/modules/rank/internal/documents"
	"gameserver/modules/rank/internal/models"
	"testing"
	"time"
)

func saveTestDocument(t *testing.T, store *memoryRankStore, data mongodb.PersistData) {
	t.Helper()
	if err := store.Save(data); err != nil {
		t.Fatal(err)
	}
}

func testBoard(rankType message.RankType, season int32, score int64) *documents.Leaderboard {
	return &documents.Leaderboard{
		PersistId: documents.BoardID(rankType, season),
		RankSnapshot: models.RankSnapshot{RankType: rankType, Season: season,
			Items: []*models.RankItem{{PlayerId: 42, Score: score, UpdateTime: rankTestNow()}}},
	}
}

func cloneTestStore(source *memoryRankStore) *memoryRankStore {
	copy := &memoryRankStore{rank: bytes.Clone(source.rank), season: bytes.Clone(source.season), boards: make(map[string][]byte)}
	for id, data := range source.boards {
		copy.boards[id] = bytes.Clone(data)
	}
	return copy
}

func TestRankMigrationResumesAndNeverOverwritesNewOrEmptyBoards(t *testing.T) {
	ladder, power, challenge := types[0], types[1], types[2]
	failure := errors.New("interrupted legacy import")
	for _, stage := range []string{"board", "marker"} {
		t.Run(stage, func(t *testing.T) {
			store := &memoryRankStore{}
			saveTestDocument(t, store, &SeasonManager{PersistId: 1, Season: 3, LastCrossDayTime: rankTestNow().Unix()})
			legacy := &documents.RankManager{PersistId: 1, RankCache: map[message.RankType]map[int32]*models.RankSnapshot{
				ladder: {1: &testBoard(ladder, 1, 1).RankSnapshot, 2: &testBoard(ladder, 2, 20).RankSnapshot,
					3: &testBoard(ladder, 3, 30).RankSnapshot, 4: &testBoard(ladder, 4, 40).RankSnapshot},
				power: {0: &testBoard(power, 0, 50).RankSnapshot}, challenge: {0: &testBoard(challenge, 0, 60).RankSnapshot},
			}}
			saveTestDocument(t, store, legacy)
			saveTestDocument(t, store, testBoard(power, 0, 999))
			empty := testBoard(challenge, 0, 0)
			empty.Items = nil
			saveTestDocument(t, store, empty)
			if stage == "board" {
				store.insertBoardErrors = map[string]error{documents.BoardID(ladder, 3): failure}
			} else {
				store.saveRankError = failure
			}
			repository := &rankRepository{store: store}
			if _, err := repository.Load(); !errors.Is(err, failure) {
				t.Fatalf("migration did not report interruption: %v", err)
			}
			metadata, err := store.LoadRank()
			if err != nil || metadata.StorageVersion != 0 || len(metadata.RankCache) == 0 {
				t.Fatalf("interrupted migration discarded legacy source: %+v %v", metadata, err)
			}
			store.insertBoardErrors, store.saveRankError = nil, nil
			// 已迁入榜的新进度必须在下一次启动中保留。
			saveTestDocument(t, store, testBoard(ladder, 2, 888))
			state, err := repository.Load()
			if err != nil {
				t.Fatal(err)
			}
			if state.metadata.StorageVersion != documents.StorageVersion || len(state.metadata.RankCache) != 0 {
				t.Fatal("migration did not switch metadata to the new layout")
			}
			if len(state.boards) != 4 {
				t.Fatalf("migration resurrected expired or future legacy season: %+v", state.boards)
			}
			for _, board := range state.boards {
				switch board.PersistId {
				case documents.BoardID(challenge, 0):
					if len(board.Items) != 0 {
						t.Fatal("legacy source overwrote an explicit empty board")
					}
				case documents.BoardID(power, 0):
					if board.Items[0].Score != 999 {
						t.Fatal("legacy source overwrote new score")
					}
				case documents.BoardID(ladder, 2):
					if board.Items[0].Score != 888 {
						t.Fatal("retry overwrote progress saved after partial migration")
					}
				case documents.BoardID(ladder, 3):
					if board.Items[0].Score != 30 || board.SnapshotTime != rankTestNow().Unix() {
						t.Fatal("legacy scores or snapshot progress were not migrated")
					}
				}
			}
			// 即使旧字段仍存在，迁移完成后缺失的新榜也不能触发回填。
			state.metadata.RankCache = legacy.RankCache
			saveTestDocument(t, store, state.metadata)
			delete(store.boards, documents.BoardID(power, 0))
			state, err = repository.Load()
			if err != nil || len(state.boards) != 3 {
				t.Fatalf("completed migration fell back to legacy data: %+v %v", state, err)
			}
			store.loadBoardsError = failure
			if _, err := repository.Load(); !errors.Is(err, failure) {
				t.Fatalf("read failure was treated as an empty board: %v", err)
			}
		})
	}
}

func TestRankPartialSaveRestoresIndependentBoardsAndCanRetry(t *testing.T) {
	store := &memoryRankStore{}
	players := fakePlayerReader{42: {}}
	baseline, baselineSystem := startTestRankManager(t, store, players, rankTestNow)
	for _, rankType := range types {
		baseline.HandleUpdateRankData(42, &message.C2S_UpdateRankData{RankType: rankType, Score: 10})
	}
	if err := baselineSystem.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	manager, system := startTestRankManager(t, store, players, rankTestNow)
	for _, rankType := range types {
		manager.HandleUpdateRankData(42, &message.C2S_UpdateRankData{RankType: rankType, Score: 100})
	}
	metadataBefore, seasonBefore := bytes.Clone(store.rank), bytes.Clone(store.season)
	first, second := errors.New("ladder unavailable"), errors.New("power unavailable")
	store.saveBoardErrors = map[string]error{
		documents.BoardID(types[0], manager.GetSeasonInfo().Season): first,
		documents.BoardID(types[1], 0):                              second,
	}
	defer func() { store.saveBoardErrors = nil }()
	if err := system.Stop(context.Background()); !errors.Is(err, first) || !errors.Is(err, second) {
		t.Fatalf("not all board failures returned: %v", err)
	}
	if !bytes.Equal(store.rank, metadataBefore) || !bytes.Equal(store.season, seasonBefore) {
		t.Fatal("failed board save advanced metadata")
	}
	if response := manager.HandleUpdateRankData(42, &message.C2S_UpdateRankData{RankType: types[2], Score: 999}); response.Success {
		t.Fatal("failed stop did not isolate the actor")
	}
	// 模拟进程退出：新实例只能读取已落盘的不同批次文档。
	restarted, _ := startTestRankManager(t, cloneTestStore(store), players, rankTestNow)
	if response := restarted.HandleGetMyRank(42, types[1], 0); response.MyScore != 10 {
		t.Fatalf("failed board did not retain old version: %v", response)
	}
	if response := restarted.HandleGetMyRank(42, types[2], 0); response.MyScore != 100 {
		t.Fatalf("successful board lost new version: %v", response)
	}
	store.saveBoardErrors = nil
	if err := system.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	retried, _ := startTestRankManager(t, cloneTestStore(store), players, rankTestNow)
	if response := retried.HandleGetMyRank(42, types[1], 0); response.MyScore != 100 {
		t.Fatalf("retry did not persist isolated actor state: %v", response)
	}
}

func TestRankRecoveryDoesNotReplaySavedDailySnapshot(t *testing.T) {
	store := &memoryRankStore{}
	players := fakePlayerReader{42: {}, 43: {}}
	baseline, system := startTestRankManager(t, store, players, rankTestNow)
	baseline.HandleUpdateRankData(42, &message.C2S_UpdateRankData{RankType: types[0], Score: 100})
	baseline.HandleUpdateRankData(43, &message.C2S_UpdateRankData{RankType: types[0], Score: 50})
	if err := system.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return rankTestNow().Add(24 * time.Hour) }
	manager, nextSystem := startTestRankManager(t, store, players, now)
	manager.HandleUpdateRankData(42, &message.C2S_UpdateRankData{RankType: types[0], Score: 1})
	failure := errors.New("season write interrupted")
	store.saveSeasonError = failure
	defer func() { store.saveSeasonError = nil }()
	if err := nextSystem.Stop(context.Background()); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	crashStore := cloneTestStore(store)
	boards, err := crashStore.LoadBoards()
	if err != nil {
		t.Fatal(err)
	}
	for _, board := range boards {
		if board.RankType == types[0] {
			// 表示榜单已保存的领奖进度；当前奖励处理本身不属于本次改造。
			board.HistoryItemCache[42].AcceptDaylyReward = true
			saveTestDocument(t, crashStore, &board)
		}
	}
	restarted, _ := startTestRankManager(t, crashStore, players, now)
	history := restarted.GetMyHistoryRank(42).Daily
	if history.LastRank != 1 || history.CurrentRank != 2 || !history.AcceptReward {
		t.Fatalf("saved daily snapshot was regenerated: %v", history)
	}
}

func TestRankRecoveryKeepsNewSeasonBoardWhenSeasonSaveFails(t *testing.T) {
	store := &memoryRankStore{}
	players := fakePlayerReader{42: {}}
	baseline, system := startTestRankManager(t, store, players, rankTestNow)
	baseline.HandleUpdateRankData(42, &message.C2S_UpdateRankData{RankType: types[0], Score: 10})
	if err := system.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return rankTestNow().AddDate(0, 0, 7) }
	manager, nextSystem := startTestRankManager(t, store, players, now)
	season := manager.GetSeasonInfo().Season
	manager.HandleUpdateRankData(42, &message.C2S_UpdateRankData{RankType: types[0], Score: 999})
	store.saveSeasonError = errors.New("season save failed")
	defer func() { store.saveSeasonError = nil }()
	if err := nextSystem.Stop(context.Background()); err == nil {
		t.Fatal("expected failed season write")
	}
	restarted, _ := startTestRankManager(t, cloneTestStore(store), players, now)
	if response := restarted.HandleGetMyRank(42, types[0], season); response.MyScore != 999 {
		t.Fatalf("new season board was dropped before season catch-up: %v", response)
	}
}

func TestRankExpiredSeasonCleanupCanRetry(t *testing.T) {
	store := &memoryRankStore{}
	saveTestDocument(t, store, &documents.RankManager{PersistId: 1, StorageVersion: documents.StorageVersion})
	saveTestDocument(t, store, &SeasonManager{PersistId: 1, Season: 3, LastCrossDayTime: rankTestNow().Unix()})
	saveTestDocument(t, store, testBoard(types[0], 1, 999))
	manager, system := startTestRankManager(t, store, fakePlayerReader{42: {}}, rankTestNow)
	if response := manager.HandleGetMyRank(42, types[0], 1); response.MyScore != 0 {
		t.Fatal("expired season was visible after load")
	}
	failure := errors.New("delete interrupted")
	store.deleteError = failure
	defer func() { store.deleteError = nil }()
	if err := system.Stop(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("cleanup error was hidden: %v", err)
	}
	store.deleteError = nil
	if err := system.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.boards[documents.BoardID(types[0], 1)]; ok {
		t.Fatal("retry retained expired document")
	}
}
