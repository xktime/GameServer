package actor

import (
	"gameserver/common/utils"
	"gameserver/core/log"
	"time"
)

type ActorTimer interface {
	GetInterval() int
	OnTimer()
}

var lastTickTime int64

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

	// 因为跨天时间可能因为配置或者时区的不一致，不能使用cronExpr。直接每秒判断是否跨天
	nowTimestamp := now.Unix()
	if lastTickTime != 0 && utils.IsCrossDay(lastTickTime, nowTimestamp) {
		OnCrossDay(nowTimestamp, actorsSnapshot)
	}
	lastTickTime = nowTimestamp
}

// OnCrossDayTimer 跨天定时器接口
type OnCrossDayTimer interface {
	// OnCrossDay 触发跨天逻辑
	OnCrossDay(timestamp int64)
	// DoOnCrossDay 执行具体的跨天逻辑，由实现类重写
	DoOnCrossDay()
	// DoOnCrossWeek 执行具体的跨周逻辑，由实现类重写
	DoOnCrossWeek()
	// GetLastCrossDayTime 获取最后一次跨天时间
	GetLastCrossDayTime() int64
	// SetLastCrossDayTime 设置最后一次跨天时间
	SetLastCrossDayTime(t int64)
}

// OnCrossDayTimerImpl 跨天定时器基础实现
type OnCrossDayTimerImpl struct {
	// LastCrossDayTime 最后一次跨天时间戳
	LastCrossDayTime int64 `bson:"last_cross_day_time"`
	// owner 持有这个实现的结构体，用于调用正确的DoOnCrossDay方法
	owner OnCrossDayTimer
}

// SetOwner 设置拥有者，必须在初始化时调用
func (c *OnCrossDayTimerImpl) SetOwner(owner OnCrossDayTimer) {
	c.owner = owner
}

// GetLastCrossDayTime 获取最后一次跨天时间
func (c *OnCrossDayTimerImpl) GetLastCrossDayTime() int64 {
	return c.LastCrossDayTime
}

// SetLastCrossDayTime 设置最后一次跨天时间
func (c *OnCrossDayTimerImpl) SetLastCrossDayTime(t int64) {
	c.LastCrossDayTime = t
}

// OnCrossDay 触发跨天逻辑
func (c *OnCrossDayTimerImpl) OnCrossDay(timestamp int64) {
	if !utils.IsCrossDay(c.GetLastCrossDayTime(), timestamp) {
		return
	}
	crossWeek := !utils.IsInSameWeek(timestamp, c.GetLastCrossDayTime())
	// 如果设置了owner，调用owner的DoOnCrossDay方法
	// 这样就能确保调用到嵌入结构体（如Shop）实现的方法
	if c.owner != nil {
		if crossWeek {
			c.owner.DoOnCrossWeek()
		}
		c.owner.DoOnCrossDay()
	} else {
		// 回退到默认实现
		if crossWeek {
			c.DoOnCrossWeek()
		}
		c.DoOnCrossDay()
	}

	c.SetLastCrossDayTime(timestamp)
}

// DoOnCrossDay 默认的跨天逻辑实现
func (c *OnCrossDayTimerImpl) DoOnCrossDay() {
	log.Error("执行的默认跨天逻辑，应该被嵌入结构体重写")
}

func (c *OnCrossDayTimerImpl) DoOnCrossWeek() {
	log.Error("执行的默认跨周逻辑，应该被嵌入结构体重写")
}

func OnCrossDay(timestamp int64, actorsSnapshot map[string]*TaskHandler) {
	for _, meta := range actorsSnapshot {
		for _, a := range meta.actors {
			if a == nil {
				continue
			}
			if timer, ok := a.(OnCrossDayTimer); ok {
				// todo 跨天是否有顺序依赖
				meta.SendTask(func() {
					timer.OnCrossDay(timestamp)
				})
			}
		}
	}
}
