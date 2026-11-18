package managers

import (
	"context"
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
	LoadRank(ctx context.Context) (*documents.RankManager, error)
	LoadSeason(ctx context.Context) (*SeasonManager, error)
	LoadBoards(ctx context.Context) ([]documents.Leaderboard, error)
	Save(context.Context, mongodb.PersistData) error
	InsertBoard(context.Context, *documents.Leaderboard) error
	DeleteBoard(context.Context, string) error
}

type rankRepository struct{ store rankDocumentStore }

type rankState struct {
	metadata *documents.RankManager
	season   *SeasonManager
	boards   []documents.Leaderboard
}

func (r *rankRepository) Load(ctx context.Context) (*rankState, error) {
	season, err := r.store.LoadSeason(ctx)
	if err != nil {
		return nil, fmt.Errorf("load season: %w", err)
	}
	if season == nil {
		season = &SeasonManager{PersistId: 1}
	}
	season.AfterLoad()
	metadata, err := r.store.LoadRank(ctx)
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
		if err := r.migrate(ctx, metadata, season); err != nil {
			return nil, err
		}
	}
	boards, err := r.store.LoadBoards(ctx)
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

func (r *rankRepository) migrate(ctx context.Context, metadata *documents.RankManager, season *SeasonManager) error {
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
			if err := r.store.InsertBoard(ctx, board); err != nil {
				failures = append(failures, fmt.Errorf("migrate leaderboard %s: %w", board.PersistId, err))
			}
		}
	}
	if err := errors.Join(failures...); err != nil {
		return err
	}
	metadata.StorageVersion = documents.StorageVersion
	metadata.RankCache = nil
	if err := r.store.Save(ctx, metadata); err != nil {
		return fmt.Errorf("finish rank migration: %w", err)
	}
	return nil
}

func (r *rankRepository) Save(ctx context.Context, state *rankState) error {
	var failures []error
	for i := range state.boards {
		board := &state.boards[i]
		if err := r.store.Save(ctx, board); err != nil {
			failures = append(failures, fmt.Errorf("save leaderboard %s: %w", board.PersistId, err))
		}
	}
	if err := errors.Join(failures...); err != nil {
		return err
	}
	if err := r.store.Save(ctx, state.metadata); err != nil {
		return fmt.Errorf("save rank metadata: %w", err)
	}
	if err := r.store.Save(ctx, state.season); err != nil {
		return fmt.Errorf("save season: %w", err)
	}
	// 只有所有活跃榜及元数据都落盘后，才清理过期赛季。
	boards, err := r.store.LoadBoards(ctx)
	if err != nil {
		return fmt.Errorf("load expired leaderboards: %w", err)
	}
	for _, board := range boards {
		if board.RankType == message.RankType_RankType_LadderPoint && board.Season < state.season.Season-1 {
			if err := r.store.DeleteBoard(ctx, board.PersistId); err != nil {
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

func (mongoRankStore) LoadRank(ctx context.Context) (*documents.RankManager, error) {
	return mongodb.FindOneByIDContext[documents.RankManager](ctx, int64(1))
}

func (mongoRankStore) LoadSeason(ctx context.Context) (*SeasonManager, error) {
	return mongodb.FindOneByIDContext[SeasonManager](ctx, int64(1))
}

func (mongoRankStore) LoadBoards(ctx context.Context) ([]documents.Leaderboard, error) {
	return mongodb.FindAllContext[documents.Leaderboard](ctx, bson.M{})
}

func (mongoRankStore) Save(ctx context.Context, data mongodb.PersistData) error {
	_, err := mongodb.SaveContext(ctx, data)
	return err
}

func (mongoRankStore) InsertBoard(ctx context.Context, board *documents.Leaderboard) error {
	_, err := mongodb.SaveIfAbsentContext(ctx, board)
	return err
}

func (mongoRankStore) DeleteBoard(ctx context.Context, id string) error {
	_, err := mongodb.DeleteByIDContext[documents.Leaderboard](ctx, id)
	return err
}
