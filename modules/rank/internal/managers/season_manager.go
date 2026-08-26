package managers

import (
	"fmt"
	"gameserver/common/db/mongodb"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	"gameserver/core/log"
)

// SeasonManager is persisted separately but mutated only inside RankManager's
// actor mailbox, avoiding a synchronous Rank <-> Season actor cycle.
type SeasonManager struct {
	PersistId        int64 `bson:"_id"`
	Season           int32 `bson:"season_id"`
	SeasonStart      int64 `bson:"season_start"`
	SeasonEnd        int64 `bson:"season_end"`
	LastCrossDayTime int64 `bson:"last_cross_day_time"`
}

func newSeasonManager() (*SeasonManager, error) {
	manager := &SeasonManager{PersistId: 1}
	if err := manager.loadFromDB(); err != nil {
		return nil, err
	}
	return manager, nil
}

func (m SeasonManager) GetPersistId() interface{} {
	return m.PersistId
}

func (m *SeasonManager) onCrossDay(rank *RankManager, now int64) {
	if !utils.IsCrossDay(m.LastCrossDayTime, now) {
		return
	}
	rank.OnCrossDay(m.Season)
	if !utils.IsInSameWeek(m.LastCrossDayTime, now) {
		m.onCrossWeek(now)
	}
	m.LastCrossDayTime = now
}

func (m *SeasonManager) onCrossWeek(now int64) {
	m.Season++
	m.SeasonStart = utils.GetWeekStart(now)
	m.SeasonEnd = utils.GetWeekEnd(now)
}

func (m *SeasonManager) loadFromDB() error {
	data, err := mongodb.FindOneById[SeasonManager](m.PersistId)
	if err != nil {
		return fmt.Errorf("load season data: %w", err)
	}
	if data != nil {
		m.Season = data.Season
		m.SeasonStart = data.SeasonStart
		m.SeasonEnd = data.SeasonEnd
		m.LastCrossDayTime = data.LastCrossDayTime
	}
	log.Debug("从数据库加载赛季数据: %v", m)
	return nil
}

func (m SeasonManager) GetSeasonInfo() *message.S2C_SeasonInfo {
	return &message.S2C_SeasonInfo{
		Season:               m.Season,
		SeasonStartTimestamp: int32(m.SeasonStart),
		SeasonEndTimestamp:   int32(m.SeasonEnd),
	}
}
