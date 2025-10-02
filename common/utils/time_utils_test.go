package utils

import (
	"testing"
	"time"
)

// TestTimeUtils_IsCrossDay 测试跨天判断功能
func TestTimeUtils_IsCrossDay(t *testing.T) {
	tu := &TimeUtils{
		config: &TimeConfig{
			DayCrossHour: 0,                             // 默认0点跨天
			Timezone:     time.FixedZone("CST", 8*3600), // 默认北京时区
		},
	}

	// 测试用例1: 同一天内的时间戳
	t.Run("同一天内", func(t *testing.T) {
		// 2023-01-01 10:00:00 和 2023-01-01 15:00:00
		timestamp1 := time.Date(2023, 1, 1, 10, 0, 0, 0, time.Local).Unix()
		timestamp2 := time.Date(2023, 1, 1, 15, 0, 0, 0, time.Local).Unix()

		if tu.IsCrossDay(timestamp1, timestamp2) {
			t.Errorf("同一天内的时间戳不应该跨天")
		}
	})

	// 测试用例2: 跨天的时间戳
	t.Run("跨天", func(t *testing.T) {
		timestamp1 := time.Date(2023, 1, 1, 23, 59, 59, 59, time.Local).Unix()
		timestamp2 := time.Date(2023, 1, 2, 0, 0, 0, 0, time.Local).Unix()

		if !tu.IsCrossDay(timestamp1, timestamp2) {
			t.Errorf("跨天的时间戳应该返回true")
		}
	})

	// 测试用例3: 自定义跨天时间点（5点跨天）
	t.Run("自定义跨天时间点", func(t *testing.T) {
		tu5 := &TimeUtils{
			config: &TimeConfig{
				DayCrossHour: 5,                             // 5点跨天
				Timezone:     time.FixedZone("CST", 8*3600), // 北京时区
			},
		}

		// 2023-01-01 04:59:59 和 2023-01-01 05:00:01
		timestamp1 := time.Date(2023, 1, 1, 4, 59, 59, 0, time.Local).Unix()
		timestamp2 := time.Date(2023, 1, 1, 5, 0, 1, 0, time.Local).Unix()

		if !tu5.IsCrossDay(timestamp1, timestamp2) {
			t.Errorf("5点跨天的时间戳应该返回true")
		}

		// 2023-01-01 03:00:00 和 2023-01-01 04:00:00 (同一天)
		timestamp3 := time.Date(2023, 1, 1, 3, 0, 0, 0, time.Local).Unix()
		timestamp4 := time.Date(2023, 1, 1, 4, 0, 0, 0, time.Local).Unix()

		if tu5.IsCrossDay(timestamp3, timestamp4) {
			t.Errorf("5点跨天配置下，3点和4点应该在同一天")
		}
	})
}

// TestTimeUtils_DaysBetween 测试天数计算功能
func TestTimeUtils_DaysBetween(t *testing.T) {
	tu := &TimeUtils{
		config: &TimeConfig{
			DayCrossHour: 0,                             // 默认0点跨天
			Timezone:     time.FixedZone("CST", 8*3600), // 北京时区
		},
	}

	// 测试用例1: 同一天
	t.Run("同一天", func(t *testing.T) {
		timestamp1 := time.Date(2023, 1, 1, 10, 0, 0, 0, time.Local).Unix()
		timestamp2 := time.Date(2023, 1, 1, 15, 0, 0, 0, time.Local).Unix()

		days := tu.DaysBetween(timestamp1, timestamp2)
		if days != 0 {
			t.Errorf("同一天的天数差应该为0，实际为%d", days)
		}
	})

	// 测试用例2: 相邻两天
	t.Run("相邻两天", func(t *testing.T) {
		timestamp1 := time.Date(2023, 1, 1, 23, 59, 59, 0, time.Local).Unix()
		timestamp2 := time.Date(2023, 1, 2, 0, 0, 1, 0, time.Local).Unix()

		days := tu.DaysBetween(timestamp1, timestamp2)
		if days != 1 {
			t.Errorf("相邻两天的天数差应该为1，实际为%d", days)
		}
	})

	// 测试用例3: 相隔多天
	t.Run("相隔多天", func(t *testing.T) {
		timestamp1 := time.Date(2023, 1, 1, 0, 0, 0, 0, time.Local).Unix()
		timestamp2 := time.Date(2023, 1, 5, 0, 0, 0, 0, time.Local).Unix()

		days := tu.DaysBetween(timestamp1, timestamp2)
		if days != 4 {
			t.Errorf("相隔4天的天数差应该为4，实际为%d", days)
		}
	})

	// 测试用例4: 时间顺序颠倒
	t.Run("时间顺序颠倒", func(t *testing.T) {
		timestamp1 := time.Date(2023, 1, 5, 0, 0, 0, 0, time.Local).Unix()
		timestamp2 := time.Date(2023, 1, 1, 0, 0, 0, 0, time.Local).Unix()

		days := tu.DaysBetween(timestamp1, timestamp2)
		if days != 4 {
			t.Errorf("时间顺序颠倒时天数差应该为4，实际为%d", days)
		}
	})
}

// TestTimeUtils_IsSameDay 测试同一天判断功能
func TestTimeUtils_IsSameDay(t *testing.T) {
	tu := &TimeUtils{
		config: &TimeConfig{
			DayCrossHour: 0,                             // 默认0点跨天
			Timezone:     time.FixedZone("CST", 8*3600), // 北京时区
		},
	}

	// 测试用例1: 同一天
	t.Run("同一天", func(t *testing.T) {
		timestamp1 := time.Date(2023, 1, 1, 10, 0, 0, 0, time.Local).Unix()
		timestamp2 := time.Date(2023, 1, 1, 15, 0, 0, 0, time.Local).Unix()

		if !tu.IsSameDay(timestamp1, timestamp2) {
			t.Errorf("同一天的时间戳应该返回true")
		}
	})

	// 测试用例2: 不同天
	t.Run("不同天", func(t *testing.T) {
		timestamp1 := time.Date(2023, 1, 1, 23, 59, 59, 0, time.Local).Unix()
		timestamp2 := time.Date(2023, 1, 2, 0, 0, 1, 0, time.Local).Unix()

		if tu.IsSameDay(timestamp1, timestamp2) {
			t.Errorf("不同天的时间戳应该返回false")
		}
	})
}

// TestTimeUtils_GetDayStart 测试获取天开始时间功能
func TestTimeUtils_GetDayStart(t *testing.T) {
	tu := &TimeUtils{
		config: &TimeConfig{
			DayCrossHour: 0,                             // 默认0点跨天
			Timezone:     time.FixedZone("CST", 8*3600), // 北京时区
		},
	}

	// 测试用例1: 0点跨天
	t.Run("0点跨天", func(t *testing.T) {
		// 2023-01-01 15:30:45
		timestamp := time.Date(2023, 1, 1, 15, 30, 45, 0, time.Local).Unix()
		dayStart := tu.GetDayStart(timestamp)

		expected := time.Date(2023, 1, 1, 0, 0, 0, 0, time.Local).Unix()
		if dayStart != expected {
			t.Errorf("天开始时间应该为%d，实际为%d", expected, dayStart)
		}
	})

	// 测试用例2: 5点跨天
	t.Run("5点跨天", func(t *testing.T) {
		tu5 := &TimeUtils{
			config: &TimeConfig{
				DayCrossHour: 5,                             // 5点跨天
				Timezone:     time.FixedZone("CST", 8*3600), // 北京时区
			},
		}

		// 2023-01-01 15:30:45 (应该属于2023-01-01 05:00:00开始的天)
		timestamp := time.Date(2023, 1, 1, 15, 30, 45, 0, time.Local).Unix()
		dayStart := tu5.GetDayStart(timestamp)

		expected := time.Date(2023, 1, 1, 5, 0, 0, 0, time.Local).Unix()
		if dayStart != expected {
			t.Errorf("5点跨天配置下，天开始时间应该为%d，实际为%d", expected, dayStart)
		}

		// 2023-01-01 03:30:45 (应该属于2022-12-31 05:00:00开始的天)
		timestamp2 := time.Date(2023, 1, 1, 3, 30, 45, 0, time.Local).Unix()
		dayStart2 := tu5.GetDayStart(timestamp2)

		expected2 := time.Date(2022, 12, 31, 5, 0, 0, 0, time.Local).Unix()
		if dayStart2 != expected2 {
			t.Errorf("5点跨天配置下，3点30分的天开始时间应该为%d，实际为%d", expected2, dayStart2)
		}
	})
}

// TestTimeUtils_GetDayEnd 测试获取天结束时间功能
func TestTimeUtils_GetDayEnd(t *testing.T) {
	tu := &TimeUtils{
		config: &TimeConfig{
			DayCrossHour: 0,                             // 默认0点跨天
			Timezone:     time.FixedZone("CST", 8*3600), // 北京时区
		},
	}

	// 测试用例1: 0点跨天
	t.Run("0点跨天", func(t *testing.T) {
		// 2023-01-01 15:30:45
		timestamp := time.Date(2023, 1, 1, 15, 30, 45, 0, time.Local).Unix()
		dayEnd := tu.GetDayEnd(timestamp)

		expected := time.Date(2023, 1, 1, 23, 59, 59, 0, time.Local).Unix()
		if dayEnd != expected {
			t.Errorf("天结束时间应该为%d，实际为%d", expected, dayEnd)
		}
	})

	// 测试用例2: 5点跨天
	t.Run("5点跨天", func(t *testing.T) {
		tu5 := &TimeUtils{
			config: &TimeConfig{
				DayCrossHour: 5,                             // 5点跨天
				Timezone:     time.FixedZone("CST", 8*3600), // 北京时区
			},
		}

		// 2023-01-01 15:30:45 (应该属于2023-01-01 05:00:00开始的天)
		timestamp := time.Date(2023, 1, 1, 15, 30, 45, 0, time.Local).Unix()
		dayEnd := tu5.GetDayEnd(timestamp)

		expected := time.Date(2023, 1, 2, 4, 59, 59, 0, time.Local).Unix()
		if dayEnd != expected {
			t.Errorf("5点跨天配置下，天结束时间应该为%d，实际为%d", expected, dayEnd)
		}
	})
}

// TestTimeUtils_IsInSameWeek 测试同周判断功能
func TestTimeUtils_IsInSameWeek(t *testing.T) {
	tu := &TimeUtils{
		config: &TimeConfig{
			DayCrossHour: 0,                             // 默认0点跨天
			Timezone:     time.FixedZone("CST", 8*3600), // 北京时区
		},
	}

	// 测试用例1: 同一周
	t.Run("同一周", func(t *testing.T) {
		// 2023-01-02 (周一) 和 2023-01-06 (周五)
		timestamp1 := time.Date(2023, 1, 2, 10, 0, 0, 0, time.Local).Unix()
		timestamp2 := time.Date(2023, 1, 6, 15, 0, 0, 0, time.Local).Unix()

		if !tu.IsInSameWeek(timestamp1, timestamp2) {
			t.Errorf("同一周的时间戳应该返回true")
		}
	})

	// 测试用例2: 不同周
	t.Run("不同周", func(t *testing.T) {
		// 2023-01-01 (周日) 和 2023-01-09 (周一，下一周)
		timestamp1 := time.Date(2023, 1, 1, 12, 0, 0, 0, time.Local).Unix()
		timestamp2 := time.Date(2023, 1, 9, 12, 0, 0, 0, time.Local).Unix()

		if tu.IsInSameWeek(timestamp1, timestamp2) {
			t.Errorf("不同周的时间戳应该返回false")
		}
	})
}

// TestTimeUtils_IsInSameMonth 测试同月判断功能
func TestTimeUtils_IsInSameMonth(t *testing.T) {
	tu := &TimeUtils{
		config: &TimeConfig{
			DayCrossHour: 0,                             // 默认0点跨天
			Timezone:     time.FixedZone("CST", 8*3600), // 北京时区
		},
	}

	// 测试用例1: 同一月
	t.Run("同一月", func(t *testing.T) {
		// 2023-01-01 和 2023-01-31
		timestamp1 := time.Date(2023, 1, 1, 10, 0, 0, 0, time.Local).Unix()
		timestamp2 := time.Date(2023, 1, 31, 15, 0, 0, 0, time.Local).Unix()

		if !tu.IsInSameMonth(timestamp1, timestamp2) {
			t.Errorf("同一月的时间戳应该返回true")
		}
	})

	// 测试用例2: 不同月
	t.Run("不同月", func(t *testing.T) {
		// 2023-01-31 和 2023-02-01
		timestamp1 := time.Date(2023, 1, 31, 12, 0, 0, 0, time.Local).Unix()
		timestamp2 := time.Date(2023, 2, 1, 12, 0, 0, 0, time.Local).Unix()

		if tu.IsInSameMonth(timestamp1, timestamp2) {
			t.Errorf("不同月的时间戳应该返回false")
		}
	})

	// 测试用例3: 不同年同月
	t.Run("不同年同月", func(t *testing.T) {
		// 2022-01-01 和 2023-01-01
		timestamp1 := time.Date(2022, 1, 1, 10, 0, 0, 0, time.Local).Unix()
		timestamp2 := time.Date(2023, 1, 1, 15, 0, 0, 0, time.Local).Unix()

		if tu.IsInSameMonth(timestamp1, timestamp2) {
			t.Errorf("不同年同月的时间戳应该返回false")
		}
	})
}

// TestGlobalFunctions 测试全局函数
func TestGlobalFunctions(t *testing.T) {
	// 测试全局函数是否正常工作
	t.Run("全局函数测试", func(t *testing.T) {
		// 2023-01-01 10:00:00 和 2023-01-01 15:00:00
		timestamp1 := time.Date(2023, 1, 1, 10, 0, 0, 0, time.Local).Unix()
		timestamp2 := time.Date(2023, 1, 1, 15, 0, 0, 0, time.Local).Unix()

		// 测试全局函数
		if IsCrossDay(timestamp1, timestamp2) {
			t.Errorf("全局函数IsCrossDay测试失败")
		}

		if DaysBetween(timestamp1, timestamp2) != 0 {
			t.Errorf("全局函数DaysBetween测试失败")
		}

		if !IsSameDay(timestamp1, timestamp2) {
			t.Errorf("全局函数IsSameDay测试失败")
		}

		dayStart := GetDayStart(timestamp1)
		expected := time.Date(2023, 1, 1, 0, 0, 0, 0, time.Local).Unix()
		if dayStart != expected {
			t.Errorf("全局函数GetDayStart测试失败")
		}

		dayEnd := GetDayEnd(timestamp1)
		expectedEnd := time.Date(2023, 1, 1, 23, 59, 59, 0, time.Local).Unix()
		if dayEnd != expectedEnd {
			t.Errorf("全局函数GetDayEnd测试失败")
		}

		if !IsInSameWeek(timestamp1, timestamp2) {
			t.Errorf("全局函数IsInSameWeek测试失败")
		}

		if !IsInSameMonth(timestamp1, timestamp2) {
			t.Errorf("全局函数IsInSameMonth测试失败")
		}
	})
}

// TestEdgeCases 测试边界情况
func TestEdgeCases(t *testing.T) {
	tu := &TimeUtils{
		config: &TimeConfig{
			DayCrossHour: 0,                             // 默认0点跨天
			Timezone:     time.FixedZone("CST", 8*3600), // 北京时区
		},
	}

	t.Run("边界情况测试", func(t *testing.T) {
		// 测试闰年2月29日
		leapYear := time.Date(2024, 2, 29, 12, 0, 0, 0, time.Local).Unix()
		nextDay := time.Date(2024, 3, 1, 12, 0, 0, 0, time.Local).Unix()

		if !tu.IsCrossDay(leapYear, nextDay) {
			t.Errorf("闰年2月29日跨天测试失败")
		}

		// 测试年末跨年
		yearEnd := time.Date(2023, 12, 31, 23, 59, 59, 0, time.Local).Unix()
		yearStart := time.Date(2024, 1, 1, 0, 0, 1, 0, time.Local).Unix()

		if !tu.IsCrossDay(yearEnd, yearStart) {
			t.Errorf("年末跨年测试失败")
		}

		// 测试夏令时边界（如果适用）
		// 注意：这个测试可能在不同时区表现不同
		springForward := time.Date(2023, 3, 12, 1, 59, 59, 0, time.Local).Unix()
		springForward2 := time.Date(2023, 3, 12, 3, 0, 1, 0, time.Local).Unix()

		// 这里只是确保函数不会崩溃，具体结果取决于时区设置
		_ = tu.IsCrossDay(springForward, springForward2)
	})
}

// BenchmarkTimeUtils 性能测试
func BenchmarkTimeUtils(b *testing.B) {
	tu := &TimeUtils{
		config: &TimeConfig{
			DayCrossHour: 0,                             // 默认0点跨天
			Timezone:     time.FixedZone("CST", 8*3600), // 北京时区
		},
	}

	timestamp1 := time.Date(2023, 1, 1, 10, 0, 0, 0, time.Local).Unix()
	timestamp2 := time.Date(2023, 1, 2, 15, 0, 0, 0, time.Local).Unix()

	b.Run("IsCrossDay", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			tu.IsCrossDay(timestamp1, timestamp2)
		}
	})

	b.Run("DaysBetween", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			tu.DaysBetween(timestamp1, timestamp2)
		}
	})

	b.Run("IsSameDay", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			tu.IsSameDay(timestamp1, timestamp2)
		}
	})

	b.Run("GetDayStart", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			tu.GetDayStart(timestamp1)
		}
	})

	b.Run("GetDayEnd", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			tu.GetDayEnd(timestamp1)
		}
	})
}
