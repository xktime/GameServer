package actor

import (
	"context"
	"errors"
	"fmt"
)

// ActorRef 是指向一个 Actor 实例的不可变引用，包含用于识别实例代次的信息。
// 同一实例按任务实际入队顺序串行执行，不保证并发等待者的入队先后。
type ActorRef struct {
	system     *ActorSystem
	scope      *Scope
	key        actorKey
	generation uint64
}

// Query 在目标邮箱内执行只读任务，调用方可按截止时间结束等待。
// 同步调用链中再次调用同一实例会返回 ErrCallCycle；Actor 内部复用逻辑应直接调用普通方法。
// 超时不撤销已接收的查询；修改领域状态应使用 Command。
func Query[R any](ctx context.Context, ref ActorRef, task func(Context) (R, error)) (R, error) {
	return call(ctx, ref, task, false)
}

// Command 的上下文只限制入队等待。任务被接收后，等待它实际完成
// 或被运行时明确拒绝，避免把观察超时当作领域操作失败。调用链检查与 Query 相同。
func Command[R any](ctx context.Context, ref ActorRef, task func(Context) (R, error)) (R, error) {
	return call(ctx, ref, task, true)
}

func call[R any](ctx context.Context, ref ActorRef, task func(Context) (R, error), untilDone bool) (R, error) {
	var zero R
	if ctx == nil {
		return zero, fmt.Errorf("actor: context is nil")
	}
	if task == nil {
		return zero, fmt.Errorf("actor: task is nil")
	}
	cell, err := ref.resolveCell()
	if err != nil {
		return zero, err
	}
	if execution, ok := actorExecution(ctx); ok {
		identity := ref.identity()
		for _, caller := range execution.chain {
			if caller == identity {
				return zero, ErrCallCycle
			}
		}
	}

	chain := actorChain(ctx)
	callContext, cancel := ref.withDefaultTimeout(ctx)
	defer cancel()
	var value R
	result := make(chan error, 1)
	taskContext := callContext
	if untilDone {
		taskContext = context.WithoutCancel(ctx)
	}
	queued := actorTask{
		ctx:    taskContext,
		chain:  chain,
		result: result,
		run: func(execution Context) error {
			var err error
			value, err = task(execution)
			return err
		},
	}
	if err := cell.admit(callContext, queued, true); err != nil {
		return zero, err
	}

	var completed error
	if untilDone {
		completed = <-result
	} else {
		select {
		case completed = <-result:
		case <-callContext.Done():
			return zero, callContext.Err()
		}
	}
	if completed != nil {
		return zero, completed
	}
	return value, nil
}

// Tell 等待任务被接收，但不等待执行完成。调用方的上下文
// 只限制入队等待；任务一旦被接收，后续仍会执行完毕。
func Tell(ctx context.Context, ref ActorRef, task func(Context) error) error {
	if ctx == nil {
		return fmt.Errorf("actor: context is nil")
	}
	if task == nil {
		return fmt.Errorf("actor: task is nil")
	}
	cell, err := ref.resolveCell()
	if err != nil {
		return err
	}

	admissionContext, cancel := ref.withDefaultTimeout(ctx)
	defer cancel()
	queued := actorTask{
		ctx: context.WithoutCancel(ctx),
		run: task,
	}
	return cell.admit(admissionContext, queued, true)
}

// TryTell 尝试立即入队，队列已满时向调用方返回错误。
func TryTell(ref ActorRef, task func(Context) error) error {
	if task == nil {
		return fmt.Errorf("actor: task is nil")
	}
	cell, err := ref.resolveCell()
	if err != nil {
		return err
	}
	queued := actorTask{
		ctx: ref.system.ctx,
		run: task,
	}
	return cell.admit(ref.system.ctx, queued, false)
}

// Stop 拒绝新任务并唤醒尚未入队的等待者，执行完已接收的任务，并等待实例终止。
func (r ActorRef) Stop(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("actor: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	cell, err := r.resolveCell()
	if errors.Is(err, ErrActorStopped) {
		return nil
	}
	if err != nil {
		return err
	}
	attempt := cell.requestStop(ctx)
	if execution, ok := actorExecution(ctx); ok && execution.current == r.identity() {
		return nil
	}
	return waitStop(ctx, attempt)
}

// ForceStop 拒绝新任务、丢弃排队任务并跳过领域停止钩子，
// 唤醒尚未入队的等待者，但仍会等待当前正在执行的任务返回。
func (r ActorRef) ForceStop(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("actor: context is nil")
	}
	cell, err := r.resolveCell()
	if errors.Is(err, ErrActorStopped) {
		return nil
	}
	if err != nil {
		return err
	}
	attempt := cell.requestForceStop()
	if execution, ok := actorExecution(ctx); ok && execution.current == r.identity() {
		return nil
	}
	select {
	case <-cell.done:
		return waitStop(ctx, attempt)
	case <-ctx.Done():
		return fmt.Errorf("%w: %w", ErrActorStopping, ctx.Err())
	}
}

func (r ActorRef) resolveCell() (*actorCell, error) {
	if r.scope == nil || r.system == nil {
		return nil, fmt.Errorf("actor: reference is not bound")
	}
	r.scope.mu.Lock()
	defer r.scope.mu.Unlock()
	entry, exists := r.scope.entries[r.key]
	if !exists || entry.cell == nil {
		return nil, ErrActorStopped
	}
	if entry.generation != r.generation {
		return nil, ErrStaleActorRef
	}
	return entry.cell, nil
}

func (r ActorRef) withDefaultTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline || r.system.timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, r.system.timeout)
}

func (r ActorRef) identity() actorIdentity {
	return actorIdentity{scope: r.scope, key: r.key, generation: r.generation}
}
