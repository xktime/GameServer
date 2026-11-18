package actor

import (
	"context"
	"fmt"
	"reflect"
	"time"
)

type actorKey struct {
	definition reflect.Type
	id         any
}

type actorEntry struct {
	ready      chan struct{}
	cell       *actorCell
	generation uint64
	err        error
	retryAfter time.Time
}

// Definition 将 Actor 类型及其标识类型绑定到作用域中注册的工厂函数。
type Definition[T Actor, ID comparable] struct {
	scope         *Scope
	key           reflect.Type
	factory       func(context.Context, ID) (T, error)
	queueCapacity int
}

// DefinitionOptions 配置一个 Actor 定义所需的运行时资源。
// QueueCapacity 为零时采用 256，为正数时使用指定容量。
type DefinitionOptions struct {
	QueueCapacity int
}

// Define 为作用域中的 Actor 类型注册唯一工厂，队列资源由定义方声明。
func Define[T Actor, ID comparable](scope *Scope, factory func(context.Context, ID) (T, error), options DefinitionOptions) (*Definition[T, ID], error) {
	if scope == nil {
		return nil, fmt.Errorf("actor: scope is nil")
	}
	if factory == nil {
		return nil, fmt.Errorf("actor: factory is nil")
	}
	if options.QueueCapacity < 0 {
		return nil, fmt.Errorf("actor: queue capacity is negative")
	}
	key := reflect.TypeOf((*T)(nil)).Elem()
	scope.mu.Lock()
	defer scope.mu.Unlock()
	if scope.stopped {
		return nil, fmt.Errorf("%w: %q", ErrScopeStopped, scope.name)
	}
	if _, exists := scope.definitions[key]; exists {
		return nil, fmt.Errorf("actor: %s is already defined in scope %q", key, scope.name)
	}
	scope.definitions[key] = struct{}{}
	queueCapacity := options.QueueCapacity
	if queueCapacity <= 0 {
		queueCapacity = defaultQueueCapacity
	}
	return &Definition[T, ID]{scope: scope, key: key, factory: factory, queueCapacity: queueCapacity}, nil
}

// GetOrCreate 返回指定标识对应的唯一就绪 Actor。调用方的上下文
// 仅控制等待过程，初始化的生命周期由 ActorSystem 管理。
func (d *Definition[T, ID]) GetOrCreate(ctx context.Context, id ID) (T, error) {
	var zero T
	if ctx == nil {
		return zero, fmt.Errorf("actor: context is nil")
	}
	key := actorKey{definition: d.key, id: id}

	d.scope.mu.Lock()
	if d.scope.stopped {
		d.scope.mu.Unlock()
		return zero, fmt.Errorf("%w: %q", ErrScopeStopped, d.scope.name)
	}
	entry, exists := d.scope.entries[key]
	if exists && entry.err != nil && !time.Now().Before(entry.retryAfter) {
		delete(d.scope.entries, key)
		entry = nil
		exists = false
	}
	if !exists {
		generation := d.scope.generations[key] + 1
		d.scope.generations[key] = generation
		entry = &actorEntry{ready: make(chan struct{}), generation: generation}
		d.scope.entries[key] = entry
		go d.initialize(key, entry, id)
	}
	d.scope.mu.Unlock()

	select {
	case <-entry.ready:
		if entry.err != nil {
			return zero, entry.err
		}
		if entry.cell == nil {
			return zero, ErrActorStopped
		}
		actor, ok := entry.cell.actor.(T)
		if !ok {
			return zero, fmt.Errorf("actor: initialized %T, want %s", entry.cell.actor, d.key)
		}
		if err := entry.cell.availabilityError(); err != nil {
			return zero, err
		}
		return actor, nil
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}

// Lookup 返回已有的就绪 Actor，不会调用定义中的工厂函数；
// 如果该实例正在初始化，则等待初始化完成。
func (d *Definition[T, ID]) Lookup(ctx context.Context, id ID) (T, error) {
	var zero T
	if ctx == nil {
		return zero, fmt.Errorf("actor: context is nil")
	}
	key := actorKey{definition: d.key, id: id}

	d.scope.mu.Lock()
	if err := ctx.Err(); err != nil {
		d.scope.mu.Unlock()
		return zero, err
	}
	if d.scope.stopped {
		d.scope.mu.Unlock()
		return zero, fmt.Errorf("%w: %q", ErrScopeStopped, d.scope.name)
	}
	entry := d.scope.entries[key]
	if entry == nil {
		d.scope.mu.Unlock()
		return zero, ErrActorNotFound
	}
	d.scope.mu.Unlock()

	select {
	case <-entry.ready:
	case <-ctx.Done():
		return zero, ctx.Err()
	}

	d.scope.mu.Lock()
	if d.scope.entries[key] != entry {
		d.scope.mu.Unlock()
		return zero, ErrActorNotFound
	}
	if entry.err != nil {
		d.scope.mu.Unlock()
		return zero, entry.err
	}
	if entry.cell == nil {
		d.scope.mu.Unlock()
		return zero, ErrActorStopped
	}
	instance, ok := entry.cell.actor.(T)
	if !ok {
		d.scope.mu.Unlock()
		return zero, fmt.Errorf("actor: initialized %T, want %s", entry.cell.actor, d.key)
	}
	cell := entry.cell
	d.scope.mu.Unlock()
	if err := cell.availabilityError(); err != nil {
		return zero, err
	}
	return instance, nil
}

// Stop 按标识停止受管理的实例，包括停止失败后等待保存重试的实例。
// 不经过只接受可运行实例的 Lookup；不存在的实例视为已经停止。
func (d *Definition[T, ID]) Stop(ctx context.Context, id ID) error {
	if ctx == nil {
		return fmt.Errorf("actor: context is nil")
	}
	key := actorKey{definition: d.key, id: id}
	d.scope.mu.Lock()
	entry := d.scope.entries[key]
	d.scope.mu.Unlock()
	if entry == nil {
		return nil
	}
	select {
	case <-entry.ready:
	case <-ctx.Done():
		return ctx.Err()
	}
	if entry.cell == nil {
		return nil
	}
	return entry.cell.ref.Stop(ctx)
}

func (d *Definition[T, ID]) initialize(key actorKey, entry *actorEntry, id ID) {
	var actor T
	var err error
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("actor: initialize %s: %v", d.key, recovered)
			}
		}()
		actor, err = d.factory(d.scope.ctx, id)
	}()

	d.scope.mu.Lock()
	if d.scope.stopped {
		err = ErrScopeStopped
	}
	if err == nil {
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					err = fmt.Errorf("actor: bind %s: %v", d.key, recovered)
				}
			}()
			if actor.ActorBase() == nil {
				err = fmt.Errorf("actor: %s must embed BaseActor", d.key)
				return
			}
			ref := ActorRef{
				system:     d.scope.system,
				scope:      d.scope,
				key:        key,
				generation: entry.generation,
			}
			actor.ActorBase().ref = ref
			entry.cell = &actorCell{
				ref:        ref,
				queue:      make(chan actorTask, d.queueCapacity),
				queueSlots: make(chan struct{}, d.queueCapacity),
				actor:      actor,
				state:      cellRunning,
				stopCh:     make(chan struct{}),
				forceCh:    make(chan struct{}),
				done:       make(chan struct{}),
				retryCh:    make(chan struct{}, 1),
			}
			go entry.cell.process()
		}()
	}
	entry.err = err
	if err != nil {
		entry.retryAfter = time.Now().Add(initializationFailureBackoff)
	}
	close(entry.ready)
	d.scope.mu.Unlock()
}
