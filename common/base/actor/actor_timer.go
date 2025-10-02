package actor

import (
	"gameserver/common/utils"
	"time"
)

type ActorTimer interface {
	GetInterval() int
	OnTimer()
}

func OnTimer() {
	actorsSnapshot := make(map[string]*TaskHandler, len(globalActorManager.taskHandlers))
	globalActorManager.mu.RLock()
	{
		// 创建副本，同时保存key和meta
		for key, meta := range globalActorManager.taskHandlers {
			actorsSnapshot[key] = meta
		}
	}
	globalActorManager.mu.RUnlock()
	now := time.Now()
	for _, meta := range actorsSnapshot {
		for _, a := range meta.actors {
			if a == nil {
				continue
			}
			if timer, ok := a.(ActorTimer); ok {
				if now.Second()%timer.GetInterval() == 0 {
					timer.OnTimer()
				}
			}
		}
	}
}

type OnCrossDayTimer interface {
	OnCrossDay()
	GetLastCrossDayTime() int64
	SetLastCrossDayTime(t int64)
}

type OnCrossDayTimerImpl struct {
	LastCrossDayTime int64 `bson:"last_cross_day_time"`
}

func (c *OnCrossDayTimerImpl) GetLastCrossDayTime() int64 {
	return c.LastCrossDayTime
}

func (c *OnCrossDayTimerImpl) SetLastCrossDayTime(t int64) {
	c.LastCrossDayTime = t
}

func (c *OnCrossDayTimerImpl) OnCrossDay() {
	// 跨天逻辑
}

func OnCrossDay() {
	actorsSnapshot := make(map[string]*TaskHandler, len(globalActorManager.taskHandlers))
	globalActorManager.mu.RLock()
	{
		// 创建副本，同时保存key和meta
		for key, meta := range globalActorManager.taskHandlers {
			actorsSnapshot[key] = meta
		}
	}
	globalActorManager.mu.RUnlock()
	now := time.Now().Unix()
	for _, meta := range actorsSnapshot {
		for _, a := range meta.actors {
			if a == nil {
				continue
			}
			if timer, ok := a.(OnCrossDayTimer); ok {
				if !utils.IsCrossDay(timer.GetLastCrossDayTime(), now) {
					continue
				}
				// todo 跨天是否有顺序依赖
				meta.SendTaskAsync(func() {
					timer.OnCrossDay()
					timer.SetLastCrossDayTime(now)
				})
			}
		}
	}
}
