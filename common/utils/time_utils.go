package utils

import (
	"time"
)

// TimeConfig 时间配置
type TimeConfig struct {
	// DayCrossHour 跨天的小时数，默认0点跨天，可以配置为5点跨天等
	DayCrossHour int
	// Timezone 时区配置，默认使用北京时区 (Asia/Shanghai)
	Timezone *time.Location
}

// TimeUtils 时间工具类
type TimeUtils struct {
	config *TimeConfig
}

// getTimezone 获取配置的时区，如果配置为空则返回北京时区
func (tu *TimeUtils) getTimezone() *time.Location {
	if tu.config.Timezone == nil {
		// 默认使用北京时区
		return time.FixedZone("CST", 8*3600)
	}
	return tu.config.Timezone
}

// IsCrossDay 判断两个时间戳是否跨天
// timestamp1: 第一个时间戳（秒）
// timestamp2: 第二个时间戳（秒）
// 返回: 是否跨天
func (tu *TimeUtils) IsCrossDay(timestamp1, timestamp2 int64) bool {
	t1 := time.Unix(timestamp1, 0)
	t2 := time.Unix(timestamp2, 0)

	// 获取两个时间对应的"天"标识
	day1 := tu.getDayIdentifier(t1)
	day2 := tu.getDayIdentifier(t2)

	// 比较天标识是否不同
	return day1 != day2
}

// DaysBetween 计算两个时间戳相距多少天
// timestamp1: 第一个时间戳（秒）
// timestamp2: 第二个时间戳（秒）
// 返回: 相距天数（绝对值）
func (tu *TimeUtils) DaysBetween(timestamp1, timestamp2 int64) int {
	t1 := time.Unix(timestamp1, 0)
	t2 := time.Unix(timestamp2, 0)

	// 获取两个时间对应的"天"标识
	day1 := tu.getDayIdentifier(t1)
	day2 := tu.getDayIdentifier(t2)

	// 计算天数差（时间戳差除以一天的秒数）
	diff := day2 - day1
	days := int(diff / 86400) // 86400秒 = 1天
	if days < 0 {
		days = -days
	}

	return days
}

// IsSameDay 判断两个时间戳是否在同一天
// timestamp1: 第一个时间戳（秒）
// timestamp2: 第二个时间戳（秒）
// 返回: 是否在同一天
func (tu *TimeUtils) IsSameDay(timestamp1, timestamp2 int64) bool {
	return !tu.IsCrossDay(timestamp1, timestamp2)
}

// GetDayStart 获取指定时间戳所在天的开始时间戳
// timestamp: 时间戳（秒）
// 返回: 当天开始的时间戳（秒）
func (tu *TimeUtils) GetDayStart(timestamp int64) int64 {
	t := time.Unix(timestamp, 0)
	dayStart := tu.getDayStartTime(t)
	return dayStart.Unix()
}

// GetDayEnd 获取指定时间戳所在天的结束时间戳
// timestamp: 时间戳（秒）
// 返回: 当天结束的时间戳（秒）
func (tu *TimeUtils) GetDayEnd(timestamp int64) int64 {
	t := time.Unix(timestamp, 0)
	dayEnd := tu.getDayEndTime(t)
	return dayEnd.Unix()
}

// IsInSameWeek 判断两个时间戳是否在同一周
// timestamp1: 第一个时间戳（秒）
// timestamp2: 第二个时间戳（秒）
// 返回: 是否在同一周
func (tu *TimeUtils) IsInSameWeek(timestamp1, timestamp2 int64) bool {
	t1 := time.Unix(timestamp1, 0)
	t2 := time.Unix(timestamp2, 0)

	// 获取周一的时间
	weekStart1 := tu.getWeekStart(t1)
	weekStart2 := tu.getWeekStart(t2)

	return weekStart1.Equal(weekStart2)
}

// IsInSameMonth 判断两个时间戳是否在同一月
// timestamp1: 第一个时间戳（秒）
// timestamp2: 第二个时间戳（秒）
// 返回: 是否在同一月
func (tu *TimeUtils) IsInSameMonth(timestamp1, timestamp2 int64) bool {
	t1 := time.Unix(timestamp1, 0)
	t2 := time.Unix(timestamp2, 0)

	return t1.Year() == t2.Year() && t1.Month() == t2.Month()
}

// getDayIdentifier 获取指定时间对应的"天"标识
// 根据配置的跨天小时数，返回一个唯一的天标识
func (tu *TimeUtils) getDayIdentifier(t time.Time) int64 {
	// 转换为配置的时区时间进行计算
	timezone := tu.getTimezone()
	localTime := t.In(timezone)

	// 获取当天跨天时间点（配置时区）
	crossTime := time.Date(localTime.Year(), localTime.Month(), localTime.Day(), tu.config.DayCrossHour, 0, 0, 0, timezone)

	// 如果当前时间小于跨天时间点，则属于前一天
	if localTime.Before(crossTime) {
		crossTime = crossTime.AddDate(0, 0, -1)
	}

	// 返回跨天时间点的Unix时间戳作为天标识
	return crossTime.Unix()
}

// getDayStartTime 获取指定时间所在天的开始时间
func (tu *TimeUtils) getDayStartTime(t time.Time) time.Time {
	// 转换为配置的时区时间进行计算
	timezone := tu.getTimezone()
	localTime := t.In(timezone)

	// 获取当天跨天时间点（配置时区）
	crossTime := time.Date(localTime.Year(), localTime.Month(), localTime.Day(), tu.config.DayCrossHour, 0, 0, 0, timezone)

	// 如果当前时间小于跨天时间点，则属于前一天
	if localTime.Before(crossTime) {
		crossTime = crossTime.AddDate(0, 0, -1)
	}

	return crossTime
}

// getDayEndTime 获取指定时间所在天的结束时间
func (tu *TimeUtils) getDayEndTime(t time.Time) time.Time {
	dayStart := tu.getDayStartTime(t)
	// 下一天的开始时间 - 1秒
	return dayStart.AddDate(0, 0, 1).Add(-time.Second)
}

// getWeekStart 获取指定时间所在周的周一0点时间
func (tu *TimeUtils) getWeekStart(t time.Time) time.Time {
	// 获取周一
	weekday := int(t.Weekday())
	if weekday == 0 { // 周日
		weekday = 7
	}

	// 计算到周一的天数差
	daysToMonday := weekday - 1

	// 获取周一0点
	monday := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	monday = monday.AddDate(0, 0, -daysToMonday)

	return monday
}

// 全局时间工具实例
var GlobalTimeUtils = &TimeUtils{
	config: &TimeConfig{
		DayCrossHour: 0,                             // 默认0点跨天
		Timezone:     time.FixedZone("CST", 8*3600), // 默认北京时区
	},
}

// 全局函数，使用默认配置

// IsCrossDay 判断两个时间戳是否跨天（使用默认配置）
func IsCrossDay(timestamp1, timestamp2 int64) bool {
	return GlobalTimeUtils.IsCrossDay(timestamp1, timestamp2)
}

// DaysBetween 计算两个时间戳相距多少天（使用默认配置）
func DaysBetween(timestamp1, timestamp2 int64) int {
	return GlobalTimeUtils.DaysBetween(timestamp1, timestamp2)
}

// IsSameDay 判断两个时间戳是否在同一天（使用默认配置）
func IsSameDay(timestamp1, timestamp2 int64) bool {
	return GlobalTimeUtils.IsSameDay(timestamp1, timestamp2)
}

// GetDayStart 获取指定时间戳所在天的开始时间戳（使用默认配置）
func GetDayStart(timestamp int64) int64 {
	return GlobalTimeUtils.GetDayStart(timestamp)
}

// GetDayEnd 获取指定时间戳所在天的结束时间戳（使用默认配置）
func GetDayEnd(timestamp int64) int64 {
	return GlobalTimeUtils.GetDayEnd(timestamp)
}

// IsInSameWeek 判断两个时间戳是否在同一周（使用默认配置）
func IsInSameWeek(timestamp1, timestamp2 int64) bool {
	return GlobalTimeUtils.IsInSameWeek(timestamp1, timestamp2)
}

// IsInSameMonth 判断两个时间戳是否在同一月（使用默认配置）
func IsInSameMonth(timestamp1, timestamp2 int64) bool {
	return GlobalTimeUtils.IsInSameMonth(timestamp1, timestamp2)
}
