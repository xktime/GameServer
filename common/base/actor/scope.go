package actor

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
)

// Scope 管理一个领域模块内的 Actor 定义和实例。
type Scope struct {
	system *ActorSystem
	name   string
	ctx    context.Context
	cancel context.CancelFunc

	mu          sync.Mutex
	definitions map[reflect.Type]struct{}
	entries     map[actorKey]*actorEntry
	generations map[actorKey]uint64
	stopped     bool
}

// Stop 禁止创建新 Actor，并有序停止该作用域内的所有 Actor。
func (s *Scope) Stop(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("actor: context is nil")
	}
	s.mu.Lock()
	s.stopped = true
	cells := make([]*actorCell, 0, len(s.entries))
	pending := make([]<-chan struct{}, 0)
	for _, entry := range s.entries {
		if entry.cell != nil {
			cells = append(cells, entry.cell)
		} else if entry.err == nil {
			pending = append(pending, entry.ready)
		}
	}
	s.mu.Unlock()
	s.cancel()

	attempts := make([]*stopAttempt, 0, len(cells))
	for _, cell := range cells {
		attempts = append(attempts, cell.requestStop(ctx))
	}
	stopErrors := make([]error, 0)
	for _, ready := range pending {
		select {
		case <-ready:
		case <-ctx.Done():
			return errors.Join(append(stopErrors, ctx.Err())...)
		}
	}
	for index, attempt := range attempts {
		err := waitStop(ctx, attempt)
		// 接入旧停止请求时，等旧预算下的保存结束后，用本次预算重试一次。
		if errors.Is(err, ErrStopFailed) && attempt.ctx.Err() != nil && ctx.Err() == nil {
			err = waitStop(ctx, cells[index].requestStop(ctx))
		}
		if err != nil {
			stopErrors = append(stopErrors, err)
		}
	}
	return errors.Join(stopErrors...)
}
