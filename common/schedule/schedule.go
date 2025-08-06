package schedule

import (
	"fmt"
	actor_manager "gameserver/core/actor"
	"gameserver/core/log"
	"gameserver/core/timer"
)

// 保存一个全局的Cron引用，以便需要时可以停止
var saveActorCron *timer.Cron
var dispatcher *timer.Dispatcher

func Init() {
	// 停止已有的定时任务（如果存在）
	if saveActorCron != nil {
		saveActorCron.Stop()
	}

	// 创建dispatcher（如果不存在）
	if dispatcher == nil {
		dispatcher = timer.NewDispatcher(100)
		// 启动dispatcher的事件循环
		go func() {
			for {
				t := <-dispatcher.ChanTimer
				if t != nil {
					t.Cb()
				}
			}
		}()
	}

	Register()
}

func Register() {
	// 启动Actor定时保存任务
	StartActorSaver(60)
}

// StartActorSaver 启动定时保存所有ActorData的任务
func StartActorSaver(interval int) {
	if interval <= 0 {
		interval = 60 // 默认60秒
	}

	RegisterIntervalSchedul(interval, actor_manager.SaveAllActorData)
	log.Release("Actor自动保存任务已启动，间隔%d秒", interval)
}

// interval: 保存间隔（秒）
func RegisterIntervalSchedul(interval int, f func()) {
	// 创建Cron表达式，每隔interval秒执行一次
	cronExprStr := fmt.Sprintf("*/%d * * * * *", interval)
	cronExpr, err := timer.NewCronExpr(cronExprStr)
	if err != nil {
		log.Error("创建Cron表达式失败: %v", err)
		return
	}

	// 设置定时任务
	saveActorCron = dispatcher.CronFunc(cronExpr, f)
}
