package actor

import (
	"context"
	"fmt"
	"gameserver/core/log"
	"runtime/debug"
	"time"
)

type actorTask struct {
	ctx        context.Context
	chain      []actorIdentity
	run        func(Context) error
	result     chan error
	enqueuedAt time.Time
}

func (c *actorCell) process() {
	defer c.finishProcessing()
	for {
		select {
		case <-c.forceCh:
			return
		default:
		}
		select {
		case <-c.forceCh:
			return
		case task := <-c.queue:
			if c.processTask(task) {
				return
			}
		case <-c.stopCh:
			if !c.drainAccepted() {
				return
			}
			for !c.finishStop() {
				select {
				case <-c.retryCh:
				case <-c.forceCh:
					return
				}
			}
			return
		}
	}
}

func (c *actorCell) drainAccepted() bool {
	for {
		select {
		case task := <-c.queue:
			if c.processTask(task) {
				return false
			}
		default:
			return true
		}
	}
}

// processTask 返回 true 时执行循环已经进入终态。
func (c *actorCell) processTask(task actorTask) bool {
	c.recordDequeued(task)
	select {
	case <-c.forceCh:
		c.failTask(task, ErrForcedStop)
		return true
	default:
	}
	return c.execute(task)
}

func (c *actorCell) admit(ctx context.Context, task actorTask, wait bool) (err error) {
	defer func() {
		if err != nil {
			c.ref.system.metrics.rejectedTasks.Add(1)
		}
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.availabilityError(); err != nil {
		return err
	}
	task.enqueuedAt = time.Now()
	if wait {
		select {
		case c.queueSlots <- struct{}{}:
		case <-c.stopCh:
			return c.availabilityError()
		case <-c.forceCh:
			return c.availabilityError()
		case <-c.done:
			return c.availabilityError()
		case <-ctx.Done():
			return ctx.Err()
		}
	} else {
		select {
		case c.queueSlots <- struct{}{}:
		default:
			return ErrQueueFull
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		<-c.queueSlots
		return err
	}
	if err := c.availabilityErrorLocked(); err != nil {
		<-c.queueSlots
		return err
	}
	// 预留保证发送不会阻塞；状态确认与提交共同构成任务接收点。
	c.queue <- task
	c.ref.system.metrics.acceptedTasks.Add(1)
	return nil
}

func (c *actorCell) discardQueued(err error) {
	for {
		select {
		case task := <-c.queue:
			c.recordDequeued(task)
			c.failTask(task, err)
		default:
			return
		}
	}
}

func (c *actorCell) execute(task actorTask) bool {
	identity := c.ref.identity()
	chain := append(append([]actorIdentity(nil), task.chain...), identity)
	execution := Context{Context: task.ctx, current: identity, chain: chain}
	execution.Context = context.WithValue(task.ctx, actorExecutionKey{}, execution)
	var err error
	panicked := false
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				panicked = true
				c.ref.system.metrics.handlerPanics.Add(1)
				err = fmt.Errorf("%w: %v", ErrHandlerPanic, recovered)
				log.Error("actor handler panic: %v\n%s", recovered, debug.Stack())
			}
		}()
		err = task.run(execution)
	}()
	if panicked {
		c.consecutivePanics++
	} else {
		c.consecutivePanics = 0
	}
	quarantined := c.consecutivePanics >= c.ref.system.panicThreshold
	if quarantined {
		c.quarantine()
	}
	if task.result != nil {
		task.result <- err
	} else if err != nil {
		log.Error("actor Tell task failed: %v", err)
	}
	return quarantined
}

func (c *actorCell) recordDequeued(task actorTask) {
	<-c.queueSlots
	c.ref.system.recordDequeued(task)
}

func (c *actorCell) failTask(task actorTask, err error) {
	if task.result != nil {
		task.result <- err
		return
	}
	log.Error("actor accepted Tell task was discarded: %v", err)
}
