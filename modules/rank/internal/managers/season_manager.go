package managers

import (
	"gameserver/common/base/actor"
	"gameserver/common/db/mongodb"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	"gameserver/core/log"
	"sync"
	"time"
)

// SeasonManager 使用TaskHandler实现，确保赛季操作按顺序执行
type SeasonManager struct {
	actor.BaseActor `bson:"-"`
	actor.OnCrossDayTimerImpl

	PersistId   int64 `bson:"_id"`
	Season      int32 `bson:"season_id"`
	SeasonStart int64 `bson:"season_start"`
	SeasonEnd   int64 `bson:"season_end"`
}

var (
	seasonManager     *SeasonManager
	seasonManagerOnce sync.Once
)

func GetSeasonManager() *SeasonManager {
	seasonManagerOnce.Do(func() {
		seasonManager = actor.RegisterActor[*SeasonManager](actor.Season, "1")
	})
	return seasonManager
}

// Init 初始化SeasonManager
func (m *SeasonManager) Init(args ...any) {
	m.loadFromDB()
	// todo 调用rank会死循环，后面处理
	m.SendTaskAsync(func() {
		m.OnCrossDay()
	})
}

// Stop 停止RankManager
func (m *SeasonManager) Stop() {
	m.RemoveActor(m)
}

// GetPersistId 获取持久化ID
func (r SeasonManager) GetPersistId() interface{} {
	return r.PersistId
}

func (r *SeasonManager) OnCrossDay() {
	now := time.Now().Unix()
	if !utils.IsCrossDay(r.GetLastCrossDayTime(), now) {
		return
	}
	// 每天复制
	rankManager := GetRankManager()
	rankManager.OnCrossDay(r.Season)
	if !utils.IsInSameWeek(r.GetLastCrossDayTime(), now) {
		r.OnCrossWeek(now)
	}
	r.SetLastCrossDayTime(now)
}

func (r *SeasonManager) GetCurrentSeason() int32 {
	return r.Season
}

func (r *SeasonManager) OnCrossWeek(now int64) {
	r.Season++
	r.SeasonStart = utils.GetWeekStart(now)
	r.SeasonEnd = utils.GetWeekEnd(now)
}

func (r *SeasonManager) loadFromDB() {
	log.Debug("排行榜管理器初始化完成")
	data, err := mongodb.FindOneById[SeasonManager](r.GetPersistId())
	if err != nil {
		log.Error("从数据库加载排行榜数据失败: %v", err)
		return
	}
	if data == nil {
		r.Season = 0
		r.SeasonStart = 0
		r.SeasonEnd = 0
	} else {
		r.Season = data.Season
		r.SeasonStart = data.SeasonStart
		r.SeasonEnd = data.SeasonEnd
	}
	r.PersistId = 1 // 使用固定ID，因为现在使用单例模式
	log.Debug("从数据库加载赛季数据: %v", r)
}

func (r SeasonManager) GetSeasonInfo() *message.S2C_SeasonInfo {
	return &message.S2C_SeasonInfo{
		Season:               r.Season,
		SeasonStartTimestamp: int32(r.SeasonStart),
		SeasonEndTimestamp:   int32(r.SeasonEnd),
	}
}
