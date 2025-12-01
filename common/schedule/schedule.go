package schedule

import (
	"gameserver/core/log"
	"time"
)

type ScheduleName string

const (
	ActorTimer    ScheduleName = "ActorTimer"
	ActorSaver    ScheduleName = "ActorSaver"
	FrameSync     ScheduleName = "FrameSync"
	RoomSyncState ScheduleName = "RoomSyncState"
)

var tickers = make(map[string]*time.Ticker)
var stopChans = make(map[string]chan struct{})

func Init() {
	RegisterAll()
}

func RegisterAll() {

}

func Register(name ScheduleName, intervalMs int64, f func()) {
	startTicker(name, "", time.Duration(intervalMs)*time.Millisecond, f)
}

func RegisterByUid(name ScheduleName, uid string, intervalMs int64, f func()) {
	startTicker(name, uid, time.Duration(intervalMs)*time.Millisecond, f)
}

func startTicker(name ScheduleName, uid string, d time.Duration, f func()) {
	uniqueId := getUniqueId(name, uid)
	if _, exists := tickers[uniqueId]; exists {
		log.Error("定时任务 %s 已运行，跳过重复启动", uniqueId)
		return
	}
	log.Release("定时任务 %s 已启动，间隔%d毫秒", uniqueId, d.Milliseconds())
	ticker := time.NewTicker(d)
	stopChan := make(chan struct{})
	tickers[uniqueId] = ticker
	stopChans[uniqueId] = stopChan

	go func() {
		defer func() {
			ticker.Stop()
			log.Release("定时任务 %s 已停止", uniqueId)
		}()

		for {
			select {
			case <-ticker.C:
				// 在 goroutine 中执行，避免阻塞 ticker
				go f()
			case <-stopChan:
				return
			}
		}
	}()
}

func StopSchedule(name ScheduleName) {
	StopScheduleByUid(name, "")
}

func StopScheduleByUid(name ScheduleName, uid string) {
	uniqueId := getUniqueId(name, uid)
	if stopChan, ok := stopChans[uniqueId]; ok {
		close(stopChan)
		delete(tickers, uniqueId)
		delete(stopChans, uniqueId)
	}
}

func getUniqueId(name ScheduleName, uid string) string {
	uniqueId := string(name)
	if uid != "" {
		uniqueId += "_" + uid
	}
	return uniqueId
}
