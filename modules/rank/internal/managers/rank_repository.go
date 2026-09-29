package managers

import (
	"errors"
	"fmt"
	"gameserver/common/db/mongodb"
	"gameserver/common/msg/message"
	"gameserver/modules/rank/internal/documents"
	"gameserver/modules/rank/internal/models"

	"go.mongodb.org/mongo-driver/bson"
)

// rankDocumentStore 的 Mongo 与 BSON 内存实现共用迁移、保存顺序和重试逻辑。
// 只在 Rank Actor 内同步使用；读写文档均不保留 Actor 状态的引用。
type rankDocumentStore interface {
	LoadRank() (*documents.RankManager, error)
	LoadSeason() (*SeasonManager, error)
	LoadBoards() ([]documents.Leaderboard, error)
	Save(mongodb.PersistData) error
	InsertBoard(*documents.Leaderboard) error
	DeleteBoard(string) error
}

type rankRepository struct{ store rankDocumentStore }

type rankState struct {
	metadata *documents.RankManager
	season   *SeasonManager
	boards   []documents.Leaderboard
}

func (r *rankRepository) Load() (*rankState, error) {
	season, err := r.store.LoadSeason()
	if err != nil {
		return nil, fmt.Errorf("load season: %w", err)
	}
	if season == nil {
		season = &SeasonManager{PersistId: 1}
	}
	season.AfterLoad()
	metadata, err := r.store.LoadRank()
	if err != nil {
		return nil, fmt.Errorf("load rank metadata: %w", err)
	}
	if metadata == nil {
		metadata = &documents.RankManager{}
	}
	metadata.AfterLoad()
	if metadata.StorageVersion > documents.StorageVersion {
		return nil, fmt.Errorf("unsupported rank storage version: %d", metadata.StorageVersion)
	}
	if metadata.StorageVersion < documents.StorageVersion {
		if err := r.migrate(metadata, season); err != nil {
			return nil, err
		}
	}
	boards, err := r.store.LoadBoards()
	if err != nil {
		return nil, fmt.Errorf("load leaderboards: %w", err)
	}
	for i := range boards {
		board := &boards[i]
		board.AfterLoad()
		if !validRankType(board.RankType) || board.PersistId != documents.BoardID(board.RankType, board.Season) ||
			(board.RankType == message.RankType_RankType_LadderPoint && board.Season <= 0) ||
			(board.RankType != message.RankType_RankType_LadderPoint && board.Season != 0) {
			return nil, fmt.Errorf("invalid leaderboard identity: %q", board.PersistId)
		}
	}
	return &rankState{metadata: metadata, season: season, boards: boards}, nil
}

func (r *rankRepository) migrate(metadata *documents.RankManager, season *SeasonManager) error {
	var failures []error
	for rankType, seasons := range metadata.RankCache {
		if !validRankType(rankType) {
			continue
		}
		for seasonID, snapshot := range seasons {
			if snapshot == nil || !retainedBoard(rankType, seasonID, season.Season) {
				continue
			}
			copy := models.RestoreRankData(*snapshot).Export()
			copy.RankType, copy.Season = rankType, seasonID
			if copy.SnapshotTime == 0 {
				copy.SnapshotTime = season.LastCrossDayTime
			}
			board := &documents.Leaderboard{PersistId: documents.BoardID(rankType, seasonID), RankSnapshot: copy}
			// 即使新文档是空榜也不能覆盖；迁移中断后只补齐缺失的文档。
			if err := r.store.InsertBoard(board); err != nil {
				failures = append(failures, fmt.Errorf("migrate leaderboard %s: %w", board.PersistId, err))
			}
		}
	}
	if err := errors.Join(failures...); err != nil {
		return err
	}
	metadata.StorageVersion = documents.StorageVersion
	metadata.RankCache = nil
	if err := r.store.Save(metadata); err != nil {
		return fmt.Errorf("finish rank migration: %w", err)
	}
	return nil
}

func (r *rankRepository) Save(manager *RankManager) error {
	var failures []error
	for _, seasons := range manager.rankCache {
		for _, rank := range seasons {
			snapshot := rank.Export()
			board := &documents.Leaderboard{
				PersistId: documents.BoardID(snapshot.RankType, snapshot.Season), RankSnapshot: snapshot,
			}
			if err := r.store.Save(board); err != nil {
				failures = append(failures, fmt.Errorf("save leaderboard %s: %w", board.PersistId, err))
			}
		}
	}
	if err := errors.Join(failures...); err != nil {
		return err
	}
	metadata := &documents.RankManager{
		PersistId: 1, StorageVersion: documents.StorageVersion, CodeIndex: manager.codeIndex,
		ChallengeCodeCache: make(map[int32]*models.ChallengeCode, len(manager.challengeCodeCache)),
	}
	for code, info := range manager.challengeCodeCache {
		if info != nil {
			copy := *info
			metadata.ChallengeCodeCache[code] = &copy
		}
	}
	if err := r.store.Save(metadata); err != nil {
		return fmt.Errorf("save rank metadata: %w", err)
	}
	season := *manager.season
	if err := r.store.Save(&season); err != nil {
		return fmt.Errorf("save season: %w", err)
	}
	// 只有所有活跃榜及元数据都落盘后，才清理过期赛季。
	boards, err := r.store.LoadBoards()
	if err != nil {
		return fmt.Errorf("load expired leaderboards: %w", err)
	}
	for _, board := range boards {
		if board.RankType == message.RankType_RankType_LadderPoint && board.Season < season.Season-1 {
			if err := r.store.DeleteBoard(board.PersistId); err != nil {
				failures = append(failures, fmt.Errorf("delete expired leaderboard %s: %w", board.PersistId, err))
			}
		}
	}
	return errors.Join(failures...)
}

func validRankType(rankType message.RankType) bool {
	for _, candidate := range types {
		if candidate == rankType {
			return true
		}
	}
	return false
}

func retainedBoard(rankType message.RankType, season, current int32) bool {
	if rankType == message.RankType_RankType_LadderPoint {
		return season > 0 && season >= current-1 && season <= current
	}
	return season == 0
}

type mongoRankStore struct{}

func (mongoRankStore) LoadRank() (*documents.RankManager, error) {
	return mongodb.FindOneById[documents.RankManager](int64(1))
}

func (mongoRankStore) LoadSeason() (*SeasonManager, error) {
	return mongodb.FindOneById[SeasonManager](int64(1))
}

func (mongoRankStore) LoadBoards() ([]documents.Leaderboard, error) {
	return mongodb.FindAll[documents.Leaderboard](bson.M{})
}

func (mongoRankStore) Save(data mongodb.PersistData) error {
	result, err := mongodb.Save(data)
	if err != nil {
		return err
	}
	if result == nil {
		return fmt.Errorf("MongoDB is not initialized")
	}
	return nil
}

func (mongoRankStore) InsertBoard(board *documents.Leaderboard) error {
	_, err := mongodb.SaveIfAbsent(board)
	return err
}

func (mongoRankStore) DeleteBoard(id string) error {
	result, err := mongodb.DeleteByID[documents.Leaderboard](id)
	if err != nil {
		return err
	}
	if result == nil {
		return fmt.Errorf("MongoDB is not initialized")
	}
	return nil
}
