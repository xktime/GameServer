package actor

import (
	"context"
	"fmt"
)

type actorExecutionKey struct{}

type actorIdentity struct {
	scope      *Scope
	key        actorKey
	generation uint64
}

// Context 携带当前 Actor 和同步调用链信息。
// 标准 context 派生操作会保留执行身份；仅供当前 handler 的同步调用链使用，
// 不得跨 goroutine 或在 handler 返回后复用。
type Context struct {
	context.Context
	current actorIdentity
	chain   []actorIdentity
}

// StopSelf 在当前任务内关闭入口，返回本次停止的完成凭据。
// 调用方在任务外 Wait，避免等待自己退出；ctx 控制本次停止钩子的 I/O。
func (c Context) StopSelf(ctx context.Context) (StopResult, error) {
	if ctx == nil {
		return StopResult{}, fmt.Errorf("actor: context is nil")
	}
	cell, err := c.currentCell()
	if err != nil {
		return StopResult{}, err
	}
	return StopResult{attempt: cell.requestStop(ctx)}, nil
}

// ForceStopSelf 关闭当前实例入口、丢弃排队任务并跳过停止钩子，返回完成凭据。
// 调用方在当前任务外 Wait，等待实例移除。
func (c Context) ForceStopSelf() (StopResult, error) {
	cell, err := c.currentCell()
	if err != nil {
		return StopResult{}, err
	}
	return StopResult{attempt: cell.requestForceStop()}, nil
}

func (c Context) currentCell() (*actorCell, error) {
	ref := ActorRef{scope: c.current.scope, key: c.current.key, generation: c.current.generation}
	if ref.scope != nil {
		ref.system = ref.scope.system
	}
	return ref.resolveCell()
}

func actorChain(ctx context.Context) []actorIdentity {
	if execution, ok := actorExecution(ctx); ok {
		return append([]actorIdentity(nil), execution.chain...)
	}
	return nil
}

func actorExecution(ctx context.Context) (Context, bool) {
	execution, ok := ctx.Value(actorExecutionKey{}).(Context)
	return execution, ok
}
