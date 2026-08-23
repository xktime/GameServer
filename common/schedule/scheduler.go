package schedule

import (
	"sync"
	"time"
)

type Job interface {
	Stop()
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

func (j *tickerJob) Stop() {
	j.stopOnce.Do(func() {
		close(j.stop)
	})
	<-j.done
}
