// Package documents 定义 Rank 的存档布局，避免暴露运行时容器。
package documents

import (
	"fmt"
	"gameserver/common/msg/message"
	"gameserver/modules/rank/internal/models"
)

const StorageVersion = 1

// RankManager 保持原集合名；RankCache 仅用于首次迁移旧嵌入榜单。
type RankManager struct {
	PersistId          int64                                               `bson:"_id"`
	StorageVersion     int                                                 `bson:"storage_version"`
	RankCache          map[message.RankType]map[int32]*models.RankSnapshot `bson:"rank_cache,omitempty"`
	ChallengeCodeCache map[int32]*models.ChallengeCode                     `bson:"challenge_code_cache,omitempty"`
	CodeIndex          int32                                               `bson:"code_index"`
}

func (r RankManager) GetPersistId() interface{} { return r.PersistId }

func (r *RankManager) AfterLoad() {
	r.PersistId = 1
	if r.RankCache == nil {
		r.RankCache = make(map[message.RankType]map[int32]*models.RankSnapshot)
	}
	if r.ChallengeCodeCache == nil {
		r.ChallengeCodeCache = make(map[int32]*models.ChallengeCode)
	}
	if r.CodeIndex < models.ChallengeCodeFloorint {
		r.CodeIndex = models.ChallengeCodeFloorint
	}
}

// Leaderboard 每个排行类型和赛季保存一份完整成绩文档。
type Leaderboard struct {
	PersistId           string `bson:"_id"`
	models.RankSnapshot `bson:",inline"`
}

func BoardID(rankType message.RankType, season int32) string {
	return fmt.Sprintf("%d:%d", rankType, season)
}

func (r Leaderboard) GetPersistId() interface{} { return r.PersistId }

func (r *Leaderboard) AfterLoad() { r.RankSnapshot.AfterLoad() }
