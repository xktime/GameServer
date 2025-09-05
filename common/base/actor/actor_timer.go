package actor

import "time"

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
