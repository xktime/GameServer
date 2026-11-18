package actor

import (
	"context"
	"fmt"
	"gameserver/core/log"
	"runtime/debug"
	"sync"
)

type cellState uint8

const (
	cellRunning cellState = iota
	cellStopping
	cellStopFailed
	cellForcing
	cellStopped
	cellQuarantined
)

// StopHook 在已接收任务执行完毕后保存领域状态。同一实例的钩子串行执行。
type StopHook interface {
	OnStop(context.Context) error
}

// stopAttempt 的结果在关闭 done 前写入，之后不再修改。
type stopAttempt struct {
	ctx  context.Context
	done chan struct{}
	err  error
}

// StopResult 始终观察同一次停止，Wait 不会触发保存重试。
type StopResult struct{ attempt *stopAttempt }

func (r StopResult) Wait(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("actor: context is nil")
	}
	return waitStop(ctx, r.attempt)
}

type actorCell struct {
	ref        ActorRef
	actor      Actor
	queue      chan actorTask
	queueSlots chan struct{} // 包括入队前的容量预留。

	// mu 保护生命周期状态和入队提交；不在锁内等待任务或执行钩子。
	mu      sync.Mutex
	state   cellState
	attempt *stopAttempt
	stopCh  chan struct{} // 首次关闭入口时关闭，唤醒等待入队者。
	forceCh chan struct{}
	retryCh chan struct{}
	done    chan struct{} // 执行循环退出时关闭一次。

	consecutivePanics int // 仅由执行循环访问。
}

func (c *actorCell) requestStop(ctx context.Context) *stopAttempt {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch c.state {
	case cellRunning:
		c.state = cellStopping
		c.attempt = &stopAttempt{ctx: ctx, done: make(chan struct{})}
		close(c.stopCh)
	case cellStopFailed:
		c.state = cellStopping
		c.attempt = &stopAttempt{ctx: ctx, done: make(chan struct{})}
		c.retryCh <- struct{}{}
	case cellForcing:
		if c.attempt == nil {
			c.attempt = &stopAttempt{ctx: ctx, done: make(chan struct{})}
		}
	case cellQuarantined:
		if c.attempt == nil {
			c.attempt = &stopAttempt{done: make(chan struct{})}
		}
		c.completeAttemptLocked(ErrActorQuarantined)
	}
	return c.attempt
}

func waitStop(ctx context.Context, attempt *stopAttempt) error {
	if attempt == nil {
		return nil
	}
	select {
	case <-attempt.done:
		return attempt.err
	default:
	}
	select {
	case <-attempt.done:
		return attempt.err
	case <-ctx.Done():
		return fmt.Errorf("%w: %w", ErrActorStopping, ctx.Err())
	}
}

func (c *actorCell) requestForceStop() *stopAttempt {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.requestForceStopLocked()
}

func (c *actorCell) requestQuarantinedStop(ctx context.Context) (*stopAttempt, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch c.state {
	case cellQuarantined, cellForcing, cellStopped:
		return c.requestForceStopLocked(), nil
	default:
		return nil, ErrActorNotQuarantined
	}
}

func (c *actorCell) requestForceStopLocked() *stopAttempt {
	if c.state == cellStopped || c.state == cellForcing {
		return c.attempt
	}
	c.state = cellForcing
	// 强制停止有自己的完成结果，不能复用已结束的失败保存结果。
	if c.attempt == nil {
		c.attempt = &stopAttempt{done: make(chan struct{})}
	} else {
		select {
		case <-c.attempt.done:
			c.attempt = &stopAttempt{done: make(chan struct{})}
		default:
		}
	}
	close(c.forceCh)
	select {
	case <-c.done:
		// 循环已经退出，另行完成清理，使调用方仍可按上下文结束等待。
		go c.finishForced()
	default:
	}
	return c.attempt
}

// finishProcessing 与强停请求在同一把锁下检查状态并关闭 done，
// 保证隔离与强停交错时，退出中的循环或强停发起方始终有一方负责清理。
func (c *actorCell) finishProcessing() {
	c.mu.Lock()
	if c.state == cellForcing {
		c.mu.Unlock()
		c.finishForced()
		c.mu.Lock()
	}
	close(c.done)
	c.mu.Unlock()
}

// finishStop 仅由执行循环调用。失败后保留循环和实例，等待下一次显式重试。
func (c *actorCell) finishStop() bool {
	c.mu.Lock()
	if c.state == cellForcing {
		c.mu.Unlock()
		return true
	}
	attempt := c.attempt
	c.mu.Unlock()

	err := c.runStopHook(attempt.ctx)
	c.mu.Lock()
	if c.state == cellForcing {
		c.mu.Unlock()
		return true
	}
	if err != nil {
		c.state = cellStopFailed
		attempt.err = fmt.Errorf("%w: %w", ErrStopFailed, err)
		close(attempt.done)
		c.mu.Unlock()
		return false
	}
	c.state = cellStopped
	c.mu.Unlock()
	c.removeAndComplete()
	return true
}

func (c *actorCell) finishForced() {
	c.discardQueued(ErrForcedStop)
	c.mu.Lock()
	c.state = cellStopped
	c.mu.Unlock()
	c.removeAndComplete()
}

func (c *actorCell) completeAttemptLocked(err error) {
	if c.attempt == nil {
		return
	}
	select {
	case <-c.attempt.done:
	default:
		c.attempt.err = err
		close(c.attempt.done)
	}
}

// removeAndComplete 在同一个注册表临界区内移除并发布完成结果，
// 避免重复处置观察到实例已移除，却早于原处置的完成通知返回成功。
func (c *actorCell) removeAndComplete() {
	c.ref.scope.mu.Lock()
	defer c.ref.scope.mu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry := c.ref.scope.entries[c.ref.key]; entry != nil && entry.cell == c {
		delete(c.ref.scope.entries, c.ref.key)
	}
	c.completeAttemptLocked(nil)
}

func (c *actorCell) quarantine() {
	c.mu.Lock()
	if c.state == cellForcing {
		c.mu.Unlock()
		return
	}
	if c.state == cellRunning {
		close(c.stopCh)
	}
	c.state = cellQuarantined
	// 隔离身份可被读取前，先固定原普通停止的结果，强停另建完成结果。
	c.completeAttemptLocked(ErrActorQuarantined)
	c.mu.Unlock()
	c.discardQueued(ErrActorQuarantined)
}

func (c *actorCell) isQuarantined() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state == cellQuarantined
}

func (c *actorCell) availabilityError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.availabilityErrorLocked()
}

func (c *actorCell) availabilityErrorLocked() error {
	switch c.state {
	case cellRunning:
		return nil
	case cellStopping, cellForcing:
		return ErrActorStopping
	case cellStopFailed:
		return c.attempt.err
	case cellQuarantined:
		return ErrActorQuarantined
	default:
		return ErrActorStopped
	}
}

func (c *actorCell) runStopHook(ctx context.Context) (err error) {
	hook, ok := c.actor.(StopHook)
	if !ok {
		return nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			c.ref.system.metrics.handlerPanics.Add(1)
			err = fmt.Errorf("%w in stop hook: %v", ErrHandlerPanic, recovered)
			log.Error("actor stop hook panic: %v\n%s", recovered, debug.Stack())
		}
	}()
	return hook.OnStop(ctx)
}
