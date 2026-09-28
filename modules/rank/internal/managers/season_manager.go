package managers

import (
	"gameserver/common/msg/message"
	"gameserver/common/utils"
)

// SeasonManager 单独持久化，但只允许在 RankManager 的 Actor 邮箱内
// 修改，避免排行榜与赛季 Actor 之间形成同步循环调用。
type SeasonManager struct {
	PersistId        int64 `bson:"_id"`
	Season           int32 `bson:"season_id"`
	SeasonStart      int64 `bson:"season_start"`
	SeasonEnd        int64 `bson:"season_end"`
	LastCrossDayTime int64 `bson:"last_cross_day_time"`
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

func (m SeasonManager) GetSeasonInfo() *message.S2C_SeasonInfo {
	return &message.S2C_SeasonInfo{
		Season:               m.Season,
		SeasonStartTimestamp: int32(m.SeasonStart),
		SeasonEndTimestamp:   int32(m.SeasonEnd),
	}
}
