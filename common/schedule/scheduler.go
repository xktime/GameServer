package schedule

import (
	"context"
	"sync"
	"time"
)

type Job interface {
	Stop(context.Context) error
}

type Scheduler interface {
	Every(time.Duration, func()) Job
}

type tickerScheduler struct{}

func NewScheduler() Scheduler {
	return tickerScheduler{}
}

func (tickerScheduler) Every(interval time.Duration, callback func()) Job {
	job := &tickerJob{
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		defer close(job.done)
		for {
			select {
			case <-ticker.C:
				select {
				case <-job.stop:
					return
				default:
				}
				callback()
			case <-job.stop:
				return
			}
		}
	}()
	return job
}

type tickerJob struct {
	stopOnce sync.Once
	stop     chan struct{}
	done     chan struct{}
}

// Stop 禁止后续回调，并在截止时间内等待当前回调结束。超时后可再次等待。
func (j *tickerJob) Stop(ctx context.Context) error {
	j.stopOnce.Do(func() {
		close(j.stop)
	})
	select {
	case <-j.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
