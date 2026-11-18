package actor

import (
	"sync/atomic"
	"time"
)

type actorMetrics struct {
	acceptedTasks  atomic.Uint64
	rejectedTasks  atomic.Uint64
	handlerPanics  atomic.Uint64
	queueWaitNanos atomic.Int64
	queueWaitMax   atomic.Int64
}

// MetricsSnapshot 是某一时刻 ActorSystem 运行指标的汇总快照。
type MetricsSnapshot struct {
	AcceptedTasks  uint64
	RejectedTasks  uint64
	HandlerPanics  uint64
	QueueDepth     int64
	QueueWaitTotal time.Duration
	QueueWaitMax   time.Duration
}

// Metrics 返回汇总的运行计数器及当前排队任务数。
func (s *ActorSystem) Metrics() MetricsSnapshot {
	s.mu.Lock()
	scopes := make([]*Scope, 0, len(s.scopes))
	for _, scope := range s.scopes {
		scopes = append(scopes, scope)
	}
	s.mu.Unlock()

	var queueDepth int64
	for _, scope := range scopes {
		scope.mu.Lock()
		for _, entry := range scope.entries {
			if entry.cell != nil {
				queueDepth += int64(len(entry.cell.queue))
			}
		}
		scope.mu.Unlock()
	}
	return MetricsSnapshot{
		AcceptedTasks:  s.metrics.acceptedTasks.Load(),
		RejectedTasks:  s.metrics.rejectedTasks.Load(),
		HandlerPanics:  s.metrics.handlerPanics.Load(),
		QueueDepth:     queueDepth,
		QueueWaitTotal: time.Duration(s.metrics.queueWaitNanos.Load()),
		QueueWaitMax:   time.Duration(s.metrics.queueWaitMax.Load()),
	}
}

func (s *ActorSystem) recordDequeued(task actorTask) {
	wait := time.Since(task.enqueuedAt)
	if wait < 0 {
		wait = 0
	}
	waitNanos := int64(wait)
	s.metrics.queueWaitNanos.Add(waitNanos)
	for {
		current := s.metrics.queueWaitMax.Load()
		if waitNanos <= current || s.metrics.queueWaitMax.CompareAndSwap(current, waitNanos) {
			return
		}
	}
}
