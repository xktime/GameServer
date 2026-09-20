package timer_test

import (
	"fmt"
	"gameserver/core/timer"
	"time"
)

func ExampleTimer() {
	d := timer.NewDispatcher(10)

	// 定时器示例一。
	d.AfterFunc(1, func() {
		fmt.Println("My name is Leaf")
	})

	// 定时器示例二。
	t := d.AfterFunc(1, func() {
		fmt.Println("will not print")
	})
	t.Stop()

	// 派发到期的定时任务。
	(<-d.ChanTimer).Cb()

	// Output:
	// My name is Leaf
}

func ExampleCronExpr() {
	cronExpr, err := timer.NewCronExpr("0 * * * *")
	if err != nil {
		return
	}

	fmt.Println(cronExpr.Next(time.Date(
		2000, 1, 1,
		20, 10, 5,
		0, time.UTC,
	)))

	// Output:
	// 2000-01-01 21:00:00 +0000 UTC
}

func ExampleCron() {
	d := timer.NewDispatcher(10)

	// 解析周期任务表达式。
	cronExpr, err := timer.NewCronExpr("* * * * * *")
	if err != nil {
		return
	}

	// 创建周期任务。
	var c *timer.Cron
	c = d.CronFunc(cronExpr, func() {
		fmt.Println("My name is Leaf")
		c.Stop()
	})

	// 派发到期的定时任务。
	(<-d.ChanTimer).Cb()

	// Output:
	// My name is Leaf
}
