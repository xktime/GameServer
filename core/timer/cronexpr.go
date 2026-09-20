package timer

// 参考实现：https://github.com/robfig/cron
import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// 字段       | 是否必填 | 允许值 | 支持的特殊字符
// ----------   | ---------- | -------------- | --------------------------
// 秒         | 否       | 0-59   | * / , -
// 分钟       | 是       | 0-59   | * / , -
// 小时       | 是       | 0-23   | * / , -
// 月内日期   | 是       | 1-31   | * / , -
// 月份       | 是       | 1-12   | * / , -
// 星期       | 是       | 0-6    | * / , -
type CronExpr struct {
	sec   uint64
	min   uint64
	hour  uint64
	dom   uint64
	month uint64
	dow   uint64
}

// 支持并发调用。
func NewCronExpr(expr string) (cronExpr *CronExpr, err error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 && len(fields) != 6 {
		err = fmt.Errorf("invalid expr %v: expected 5 or 6 fields, got %v", expr, len(fields))
		return
	}

	if len(fields) == 5 {
		fields = append([]string{"0"}, fields...)
	}

	cronExpr = new(CronExpr)
	// 秒
	cronExpr.sec, err = parseCronField(fields[0], 0, 59)
	if err != nil {
		goto onError
	}
	// 分钟
	cronExpr.min, err = parseCronField(fields[1], 0, 59)
	if err != nil {
		goto onError
	}
	// 小时
	cronExpr.hour, err = parseCronField(fields[2], 0, 23)
	if err != nil {
		goto onError
	}
	// 月内日期
	cronExpr.dom, err = parseCronField(fields[3], 1, 31)
	if err != nil {
		goto onError
	}
	// 月份
	cronExpr.month, err = parseCronField(fields[4], 1, 12)
	if err != nil {
		goto onError
	}
	// 星期
	cronExpr.dow, err = parseCronField(fields[5], 0, 6)
	if err != nil {
		goto onError
	}
	return

onError:
	err = fmt.Errorf("invalid expr %v: %v", expr, err)
	return
}

// 1. *：匹配该字段的全部取值。
// 2. num：匹配单个数值。
// 3. num-num：匹配闭区间内的数值。
// 4. */num：在完整取值范围内按指定步长匹配。
// 5. num/num：从指定起点按步长匹配，等价于 num-max/num。
// 6. num-num/num：在指定区间内按步长匹配。
func parseCronField(field string, min int, max int) (cronField uint64, err error) {
	fields := strings.Split(field, ",")
	for _, field := range fields {
		rangeAndIncr := strings.Split(field, "/")
		if len(rangeAndIncr) > 2 {
			err = fmt.Errorf("too many slashes: %v", field)
			return
		}

		// 解析取值区间。
		startAndEnd := strings.Split(rangeAndIncr[0], "-")
		if len(startAndEnd) > 2 {
			err = fmt.Errorf("too many hyphens: %v", rangeAndIncr[0])
			return
		}

		var start, end int
		if startAndEnd[0] == "*" {
			if len(startAndEnd) != 1 {
				err = fmt.Errorf("invalid range: %v", rangeAndIncr[0])
				return
			}
			start = min
			end = max
		} else {
			// 区间起点。
			start, err = strconv.Atoi(startAndEnd[0])
			if err != nil {
				err = fmt.Errorf("invalid range: %v", rangeAndIncr[0])
				return
			}
			// 区间终点。
			if len(startAndEnd) == 1 {
				if len(rangeAndIncr) == 2 {
					end = max
				} else {
					end = start
				}
			} else {
				end, err = strconv.Atoi(startAndEnd[1])
				if err != nil {
					err = fmt.Errorf("invalid range: %v", rangeAndIncr[0])
					return
				}
			}
		}

		if start > end {
			err = fmt.Errorf("invalid range: %v", rangeAndIncr[0])
			return
		}
		if start < min {
			err = fmt.Errorf("out of range [%v, %v]: %v", min, max, rangeAndIncr[0])
			return
		}
		if end > max {
			err = fmt.Errorf("out of range [%v, %v]: %v", min, max, rangeAndIncr[0])
			return
		}

		// 解析步长。
		var incr int
		if len(rangeAndIncr) == 1 {
			incr = 1
		} else {
			incr, err = strconv.Atoi(rangeAndIncr[1])
			if err != nil {
				err = fmt.Errorf("invalid increment: %v", rangeAndIncr[1])
				return
			}
			if incr <= 0 {
				err = fmt.Errorf("invalid increment: %v", rangeAndIncr[1])
				return
			}
		}

		// 构造当前字段的允许值集合。
		if incr == 1 {
			cronField |= ^(math.MaxUint64 << uint(end+1)) & (math.MaxUint64 << uint(start))
		} else {
			for i := start; i <= end; i += incr {
				cronField |= 1 << uint(i)
			}
		}
	}

	return
}

func (e *CronExpr) matchDay(t time.Time) bool {
	// 月内日期未受限制时，仅检查星期条件。
	if e.dom == 0xfffffffe {
		return 1<<uint(t.Weekday())&e.dow != 0
	}

	// 星期未受限制时，仅检查月内日期条件。
	if e.dow == 0x7f {
		return 1<<uint(t.Day())&e.dom != 0
	}

	return 1<<uint(t.Weekday())&e.dow != 0 ||
		1<<uint(t.Day())&e.dom != 0
}

// 支持并发调用。
func (e *CronExpr) Next(t time.Time) time.Time {
	// 从下一秒开始查找匹配时间。
	t = t.Truncate(time.Second).Add(time.Second)

	year := t.Year()
	initFlag := false

retry:
	// 年份
	if t.Year() > year+1 {
		return time.Time{}
	}

	// 月份
	for 1<<uint(t.Month())&e.month == 0 {
		if !initFlag {
			initFlag = true
			t = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
		}

		t = t.AddDate(0, 1, 0)
		if t.Month() == time.January {
			goto retry
		}
	}

	// 日期
	for !e.matchDay(t) {
		if !initFlag {
			initFlag = true
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		}

		t = t.AddDate(0, 0, 1)
		if t.Day() == 1 {
			goto retry
		}
	}

	// 小时
	for 1<<uint(t.Hour())&e.hour == 0 {
		if !initFlag {
			initFlag = true
			t = t.Truncate(time.Hour)
		}

		t = t.Add(time.Hour)
		if t.Hour() == 0 {
			goto retry
		}
	}

	// 分钟
	for 1<<uint(t.Minute())&e.min == 0 {
		if !initFlag {
			initFlag = true
			t = t.Truncate(time.Minute)
		}

		t = t.Add(time.Minute)
		if t.Minute() == 0 {
			goto retry
		}
	}

	// 秒
	for 1<<uint(t.Second())&e.sec == 0 {
		if !initFlag {
			initFlag = true
		}

		t = t.Add(time.Second)
		if t.Second() == 0 {
			goto retry
		}
	}

	return t
}
