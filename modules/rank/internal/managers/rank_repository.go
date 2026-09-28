package managers

import (
	"fmt"
	"gameserver/common/db/mongodb"
)

// rankRepository 只在 Rank Actor 内同步使用；加载返回独立文档，保存不保留 Actor 指针。
type rankRepository interface {
	LoadRank() (*RankManager, error)
	LoadSeason() (*SeasonManager, error)
	SaveRank(*RankManager) error
	SaveSeason(*SeasonManager) error
}

type mongoRankRepository struct{}

func (mongoRankRepository) LoadRank() (*RankManager, error) {
	return mongodb.FindOneById[RankManager](int64(1))
}

func (mongoRankRepository) LoadSeason() (*SeasonManager, error) {
	return mongodb.FindOneById[SeasonManager](int64(1))
}

func (mongoRankRepository) SaveRank(data *RankManager) error {
	return saveRankDocument("rank", data)
}

func (mongoRankRepository) SaveSeason(data *SeasonManager) error {
	return saveRankDocument("season", data)
}

func saveRankDocument(name string, data mongodb.PersistData) error {
	result, err := mongodb.Save(data)
	if err != nil {
		return fmt.Errorf("save %s data: %w", name, err)
	}
	if result == nil {
		return fmt.Errorf("save %s data: MongoDB is not initialized", name)
	}
	return nil
}
