package utils

import (
	"fmt"
	"time"
)

func main() {
	DebugWeekTest()
}

func DebugWeekTest() {
	// 2023-01-01 (周日) 和 2023-01-02 (周一)
	timestamp1 := time.Date(2023, 1, 1, 23, 59, 59, 0, time.UTC).Unix()
	timestamp2 := time.Date(2023, 1, 2, 0, 0, 1, 0, time.UTC).Unix()

	t1 := time.Unix(timestamp1, 0)
	t2 := time.Unix(timestamp2, 0)

	fmt.Printf("时间1: %s, 星期: %d\n", t1.Format("2006-01-02 15:04:05 Monday"), t1.Weekday())
	fmt.Printf("时间2: %s, 星期: %d\n", t2.Format("2006-01-02 15:04:05 Monday"), t2.Weekday())

	tu := &TimeUtils{
		config: &TimeConfig{
			DayCrossHour: 0,
		},
	}

	weekStart1 := tu.getWeekStart(t1)
	weekStart2 := tu.getWeekStart(t2)

	fmt.Printf("周开始1: %s\n", weekStart1.Format("2006-01-02 15:04:05 Monday"))
	fmt.Printf("周开始2: %s\n", weekStart2.Format("2006-01-02 15:04:05 Monday"))
	fmt.Printf("是否同一周: %t\n", weekStart1.Equal(weekStart2))
}
