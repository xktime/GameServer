package actor

import (
	"context"
	"errors"
	"fmt"
	"gameserver/core/log"
	"reflect"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

const (
	initializationFailureBackoff = 100 * time.Millisecond
	defaultQueueCapacity         = 10000
	defaultPanicThreshold        = 3
	defaultQuarantineDuration    = 30 * time.Second
)

// ActorSystem owns actor scopes and their shared lifecycle.
type ActorSystem struct {
	ctx                context.Context
	cancel             context.CancelFunc
	timeout            time.Duration
	queueCapacity      int
	panicThreshold     int
	quarantineDuration time.Duration

	mu      sync.Mutex
	scopes  map[string]*Scope
	stopped bool
	metrics actorMetrics
}

type actorMetrics struct {
	acceptedTasks  atomic.Uint64
	rejectedTasks  atomic.Uint64
	handlerPanics  atomic.Uint64
	queueWaitNanos atomic.Int64
	queueWaitMax   atomic.Int64
}

// MetricsSnapshot is an aggregate point-in-time view of one ActorSystem.
type MetricsSnapshot struct {
	AcceptedTasks  uint64
	RejectedTasks  uint64
	HandlerPanics  uint64
	QueueDepth     int64
	QueueWaitTotal time.Duration
	QueueWaitMax   time.Duration
}

// Scope owns the actor definitions and instances for one domain module.
type Scope struct {
	system *ActorSystem
	name   string
	ctx    context.Context
	cancel context.CancelFunc

	mu          sync.Mutex
	definitions map[definitionKey]struct{}
	entries     map[actorKey]*actorEntry
	generations map[actorKey]uint64
	stopped     bool
}

type definitionKey struct {
	group     ActorGroup
	actorType reflect.Type
}

type actorKey struct {
	definition definitionKey
	id         any
}

type actorEntry struct {
	ready            chan struct{}
	actor            any
	cell             *actorCell
	generation       uint64
	err              error
	completed        bool
	retryAfter       time.Time
	quarantined      bool
	quarantinedUntil time.Time
}

type actorCell struct {
	ref   ActorRef
	queue chan actorTask
	actor any

	admitMu           sync.RWMutex
	lifecycleMu       sync.Mutex
	state             cellState
	stopCh            chan struct{}
	forceCh           chan struct{}
	done              chan struct{}
	stopCtx           context.Context
	stopErr           error
	consecutivePanics int
}

type cellState uint8

const (
	cellRunning cellState = iota
	cellStopping
	cellForcing
	cellStopped
	cellStopFailed
	cellQuarantined
)

// StopHook lets a domain actor flush its own state after accepted tasks drain.
type StopHook interface {
	OnStop(context.Context) error
}

type actorTask struct {
	ctx        context.Context
	chain      []actorIdentity
	run        func(Context) (any, error)
	result     chan taskResult
	enqueuedAt time.Time
}

type taskResult struct {
	value any
	err   error
}

type actorIdentity struct {
	key        actorKey
	generation uint64
}

// ActorRef is an immutable, generation-aware reference to one actor instance.
type ActorRef struct {
	system     *ActorSystem
	scope      *Scope
	key        actorKey
	generation uint64
}

// Context carries the current actor and synchronous call chain.
type Context struct {
	context.Context
	current actorIdentity
	chain   []actorIdentity
}

type actorBaseProvider interface {
	ActorBase() *BaseActor
}

// Definition binds one actor type and ID type to a factory registered in a scope.
type Definition[T any, ID comparable] struct {
	scope   *Scope
	key     definitionKey
	factory func(context.Context, ID) (T, error)
}

// NewActorSystem creates an explicit actor runtime owner.
func NewActorSystem(timeout time.Duration) *ActorSystem {
	ctx, cancel := context.WithCancel(context.Background())
	return &ActorSystem{
		ctx:                ctx,
		cancel:             cancel,
		timeout:            timeout,
		queueCapacity:      defaultQueueCapacity,
		panicThreshold:     defaultPanicThreshold,
		quarantineDuration: defaultQuarantineDuration,
		scopes:             make(map[string]*Scope),
	}
}

// NewScope creates a uniquely named domain scope.
func (s *ActorSystem) NewScope(name string) (*Scope, error) {
	if name == "" {
		return nil, fmt.Errorf("actor: scope name is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return nil, ErrSystemStopped
	}
	if _, exists := s.scopes[name]; exists {
		return nil, fmt.Errorf("actor: scope %q already exists", name)
	}
	scopeContext, cancel := context.WithCancel(s.ctx)
	scope := &Scope{
		system:      s,
		name:        name,
		ctx:         scopeContext,
		cancel:      cancel,
		definitions: make(map[definitionKey]struct{}),
		entries:     make(map[actorKey]*actorEntry),
		generations: make(map[actorKey]uint64),
	}
	s.scopes[name] = scope
	return scope, nil
}

// Stop gracefully closes every scope before canceling the runtime context.
func (s *ActorSystem) Stop(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("actor: context is nil")
	}
	s.mu.Lock()
	s.stopped = true
	scopes := make([]*Scope, 0, len(s.scopes))
	for _, scope := range s.scopes {
		scopes = append(scopes, scope)
	}
	s.mu.Unlock()

	stopErrors := make([]error, 0)
	for _, scope := range scopes {
		if err := scope.Stop(ctx); err != nil {
			stopErrors = append(stopErrors, err)
		}
	}
	s.cancel()
	return errors.Join(stopErrors...)
}

// Metrics returns aggregate runtime counters and the current queued task count.
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

// Define registers the only factory for an actor type in a scope and group.
func Define[T any, ID comparable](scope *Scope, group ActorGroup, factory func(context.Context, ID) (T, error)) (*Definition[T, ID], error) {
	if scope == nil {
		return nil, fmt.Errorf("actor: scope is nil")
	}
	if factory == nil {
		return nil, fmt.Errorf("actor: factory is nil")
	}
	key := definitionKey{
		group:     group,
		actorType: reflect.TypeOf((*T)(nil)).Elem(),
	}
	scope.mu.Lock()
	defer scope.mu.Unlock()
	if scope.stopped {
		return nil, fmt.Errorf("%w: %q", ErrScopeStopped, scope.name)
	}
	for defined := range scope.definitions {
		if defined.actorType == key.actorType {
			return nil, fmt.Errorf("actor: %s is already defined in scope %q", key.actorType, scope.name)
		}
	}
	scope.definitions[key] = struct{}{}
	return &Definition[T, ID]{scope: scope, key: key, factory: factory}, nil
}

// GetOrCreate returns the single ready actor for an identity. The caller's
// context only controls waiting; initialization belongs to the ActorSystem.
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
	if exists && entry.quarantined {
		if time.Now().Before(entry.quarantinedUntil) {
			d.scope.mu.Unlock()
			return zero, ErrActorQuarantined
		}
		delete(d.scope.entries, key)
		entry = nil
		exists = false
	}
	if exists && entry.completed && entry.err != nil && !time.Now().Before(entry.retryAfter) {
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
		actor, ok := entry.actor.(T)
		if !ok {
			return zero, fmt.Errorf("actor: initialized %T, want %s", entry.actor, d.key.actorType)
		}
		if entry.cell == nil {
			return zero, ErrActorStopped
		}
		if err := entry.cell.availabilityError(); err != nil {
			return zero, err
		}
		return actor, nil
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}

// Lookup returns an existing ready actor without invoking the definition's
// factory. It waits for an already-started initialization to finish.
func (d *Definition[T, ID]) Lookup(ctx context.Context, id ID) (T, error) {
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
	entry := d.scope.entries[key]
	if entry == nil {
		d.scope.mu.Unlock()
		return zero, ErrActorStopped
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
		return zero, ErrActorStopped
	}
	if entry.err != nil {
		d.scope.mu.Unlock()
		return zero, entry.err
	}
	if entry.cell == nil {
		d.scope.mu.Unlock()
		return zero, ErrActorStopped
	}
	if entry.quarantined {
		d.scope.mu.Unlock()
		return zero, ErrActorQuarantined
	}
	instance, ok := entry.actor.(T)
	if !ok {
		d.scope.mu.Unlock()
		return zero, fmt.Errorf("actor: initialized %T, want %s", entry.actor, d.key.actorType)
	}
	cell := entry.cell
	d.scope.mu.Unlock()
	if err := cell.availabilityError(); err != nil {
		return zero, err
	}
	return instance, nil
}

func (d *Definition[T, ID]) initialize(key actorKey, entry *actorEntry, id ID) {
	var actor T
	var err error
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("actor: initialize %s: %v", d.key.actorType, recovered)
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
					err = fmt.Errorf("actor: bind %s: %v", d.key.actorType, recovered)
				}
			}()
			provider, ok := any(actor).(actorBaseProvider)
			if !ok || provider.ActorBase() == nil {
				err = fmt.Errorf("actor: %s must embed BaseActor", d.key.actorType)
				return
			}
			ref := ActorRef{
				system:     d.scope.system,
				scope:      d.scope,
				key:        key,
				generation: entry.generation,
			}
			provider.ActorBase().ref = ref
			entry.cell = &actorCell{
				ref:     ref,
				queue:   make(chan actorTask, d.scope.system.queueCapacity),
				actor:   actor,
				state:   cellRunning,
				stopCh:  make(chan struct{}),
				forceCh: make(chan struct{}),
				done:    make(chan struct{}),
			}
			go entry.cell.process()
		}()
	}
	entry.actor = actor
	entry.err = err
	entry.completed = true
	if err != nil {
		entry.retryAfter = time.Now().Add(initializationFailureBackoff)
	}
	close(entry.ready)
	d.scope.mu.Unlock()
}

// Stop prevents new actors and gracefully stops every actor owned by the scope.
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
		} else if !entry.completed {
			pending = append(pending, entry.ready)
		}
	}
	s.mu.Unlock()
	s.cancel()

	for _, ready := range pending {
		select {
		case <-ready:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	for _, cell := range cells {
		if cell.isStopFailed() {
			continue
		}
		cell.requestStop(ctx)
	}
	stopErrors := make([]error, 0)
	for _, cell := range cells {
		if cell.isStopFailed() {
			if err := cell.retryStop(ctx); err != nil {
				stopErrors = append(stopErrors, err)
			}
			continue
		}
		select {
		case <-cell.done:
			if cell.stopErr != nil {
				stopErrors = append(stopErrors, cell.stopErr)
			}
		case <-ctx.Done():
			stopErrors = append(stopErrors, ctx.Err())
			return errors.Join(stopErrors...)
		}
	}
	return errors.Join(stopErrors...)
}

// Call enqueues a task and waits for its result.
func Call[R any](ctx context.Context, ref ActorRef, task func(Context) (R, error)) (R, error) {
	var zero R
	if ctx == nil {
		return zero, fmt.Errorf("actor: context is nil")
	}
	if task == nil {
		return zero, fmt.Errorf("actor: task is nil")
	}
	entry, err := ref.resolve()
	if err != nil {
		return zero, err
	}
	if execution, ok := actorExecution(ctx); ok {
		identity := ref.identity()
		if execution.current == identity {
			return task(execution)
		}
		for _, caller := range execution.chain {
			if caller == identity {
				return zero, ErrCallCycle
			}
		}
	}

	chain := actorChain(ctx)
	callContext, cancel := ref.withDefaultTimeout(ctx)
	defer cancel()
	result := make(chan taskResult, 1)
	queued := actorTask{
		ctx:    callContext,
		chain:  chain,
		result: result,
		run: func(execution Context) (any, error) {
			return task(execution)
		},
	}
	if err := entry.cell.admit(callContext, queued, true); err != nil {
		return zero, err
	}

	select {
	case completed := <-result:
		if completed.err != nil {
			return zero, completed.err
		}
		value, ok := completed.value.(R)
		if !ok {
			return zero, fmt.Errorf("actor: task returned %T", completed.value)
		}
		return value, nil
	case <-callContext.Done():
		return zero, fmt.Errorf("%w: %v", ErrOutcomeUnknown, callContext.Err())
	}
}

// Tell waits until a task is accepted but does not wait for its execution. The
// caller's context bounds admission; an accepted task is still drained later.
func Tell(ctx context.Context, ref ActorRef, task func(Context) error) error {
	if ctx == nil {
		return fmt.Errorf("actor: context is nil")
	}
	if task == nil {
		return fmt.Errorf("actor: task is nil")
	}
	entry, err := ref.resolve()
	if err != nil {
		return err
	}

	admissionContext, cancel := ref.withDefaultTimeout(ctx)
	defer cancel()
	queued := actorTask{
		ctx: context.WithoutCancel(ctx),
		run: func(execution Context) (any, error) {
			return nil, task(execution)
		},
	}
	return entry.cell.admit(admissionContext, queued, true)
}

// TryTell attempts immediate admission and reports a full queue to the caller.
func TryTell(ref ActorRef, task func(Context) error) error {
	if task == nil {
		return fmt.Errorf("actor: task is nil")
	}
	entry, err := ref.resolve()
	if err != nil {
		return err
	}
	queued := actorTask{
		ctx: ref.system.ctx,
		run: func(execution Context) (any, error) {
			return nil, task(execution)
		},
	}
	return entry.cell.admit(ref.system.ctx, queued, false)
}

// Stop rejects new tasks, drains accepted tasks, and waits for termination.
func (r ActorRef) Stop(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("actor: context is nil")
	}
	cell, err := r.resolveCell()
	if errorsIsActorStopped(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if cell.isStopFailed() {
		return cell.retryStop(ctx)
	}
	cell.requestStop(ctx)
	if execution, ok := actorExecution(ctx); ok && execution.current == r.identity() {
		return nil
	}
	select {
	case <-cell.done:
		return cell.stopErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ForceStop rejects new tasks, discards queued tasks, skips the domain stop
// hook, and waits for the currently executing task to return.
func (r ActorRef) ForceStop(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("actor: context is nil")
	}
	cell, err := r.resolveCell()
	if errorsIsActorStopped(err) {
		return nil
	}
	if err != nil {
		return err
	}
	select {
	case <-cell.done:
		cell.forceRemove()
		return nil
	default:
	}
	cell.requestForceStop()
	if execution, ok := actorExecution(ctx); ok && execution.current == r.identity() {
		return nil
	}
	select {
	case <-cell.done:
		cell.forceRemove()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r ActorRef) resolve() (*actorEntry, error) {
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
	return entry, nil
}

func (r ActorRef) resolveCell() (*actorCell, error) {
	entry, err := r.resolve()
	if err != nil {
		return nil, err
	}
	return entry.cell, nil
}

func (r ActorRef) withDefaultTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline || r.system.timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, r.system.timeout)
}

func actorChain(ctx context.Context) []actorIdentity {
	if execution, ok := actorExecution(ctx); ok {
		return append([]actorIdentity(nil), execution.chain...)
	}
	return nil
}

func actorExecution(ctx context.Context) (Context, bool) {
	switch execution := ctx.(type) {
	case Context:
		return execution, true
	case *Context:
		return *execution, true
	default:
		return Context{}, false
	}
}

func (r ActorRef) identity() actorIdentity {
	return actorIdentity{key: r.key, generation: r.generation}
}

func (c *actorCell) process() {
	for {
		select {
		case <-c.forceCh:
			c.finishForced()
			return
		default:
		}
		select {
		case <-c.forceCh:
			c.finishForced()
			return
		case task := <-c.queue:
			c.ref.system.recordDequeued(task)
			select {
			case <-c.forceCh:
				c.failTask(task, ErrForcedStop)
				c.finishForced()
				return
			default:
			}
			if c.execute(task) {
				c.quarantine()
				return
			}
		case <-c.stopCh:
			for {
				select {
				case <-c.forceCh:
					c.finishForced()
					return
				default:
				}
				select {
				case <-c.forceCh:
					c.finishForced()
					return
				case task := <-c.queue:
					c.ref.system.recordDequeued(task)
					select {
					case <-c.forceCh:
						c.failTask(task, ErrForcedStop)
						c.finishForced()
						return
					default:
					}
					if c.execute(task) {
						c.quarantine()
						return
					}
				default:
					c.finish()
					return
				}
			}
		}
	}
}

func (c *actorCell) execute(task actorTask) bool {
	identity := actorIdentity{key: c.ref.key, generation: c.ref.generation}
	chain := append(append([]actorIdentity(nil), task.chain...), identity)
	execution := Context{Context: task.ctx, current: identity, chain: chain}
	var value any
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
		value, err = task.run(execution)
	}()
	if panicked {
		c.consecutivePanics++
	} else {
		c.consecutivePanics = 0
	}
	if task.result != nil {
		task.result <- taskResult{value: value, err: err}
	} else if err != nil {
		log.Error("actor Tell task failed: %v", err)
	}
	return c.consecutivePanics >= c.ref.system.panicThreshold
}

func (c *actorCell) admit(ctx context.Context, task actorTask, wait bool) error {
	c.admitMu.RLock()
	defer c.admitMu.RUnlock()
	if c.state != cellRunning {
		c.ref.system.metrics.rejectedTasks.Add(1)
		if c.state == cellQuarantined {
			return ErrActorQuarantined
		}
		return ErrActorStopped
	}
	task.enqueuedAt = time.Now()
	if !wait {
		select {
		case c.queue <- task:
			c.ref.system.metrics.acceptedTasks.Add(1)
			return nil
		default:
			c.ref.system.metrics.rejectedTasks.Add(1)
			return ErrQueueFull
		}
	}
	select {
	case c.queue <- task:
		c.ref.system.metrics.acceptedTasks.Add(1)
		return nil
	case <-ctx.Done():
		c.ref.system.metrics.rejectedTasks.Add(1)
		return ctx.Err()
	}
}

func (c *actorCell) requestStop(ctx context.Context) {
	c.admitMu.Lock()
	defer c.admitMu.Unlock()
	if c.state != cellRunning {
		return
	}
	c.state = cellStopping
	c.stopCtx = ctx
	close(c.stopCh)
}

func (c *actorCell) requestForceStop() {
	c.admitMu.Lock()
	defer c.admitMu.Unlock()
	if c.state == cellStopped || c.state == cellForcing {
		return
	}
	c.state = cellForcing
	close(c.forceCh)
}

func (c *actorCell) finish() {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	c.admitMu.RLock()
	forcing := c.state == cellForcing
	c.admitMu.RUnlock()
	if forcing {
		c.finishForcedLocked()
		return
	}
	c.stopErr = c.runStopHook(c.stopCtx)
	c.admitMu.Lock()
	if c.stopErr != nil {
		c.state = cellStopFailed
	} else {
		c.state = cellStopped
	}
	c.admitMu.Unlock()

	if c.stopErr == nil {
		c.ref.scope.mu.Lock()
		if entry := c.ref.scope.entries[c.ref.key]; entry != nil && entry.cell == c {
			delete(c.ref.scope.entries, c.ref.key)
		}
		c.ref.scope.mu.Unlock()
	}
	close(c.done)
}

func (c *actorCell) finishForced() {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	c.finishForcedLocked()
}

func (c *actorCell) finishForcedLocked() {
	c.admitMu.Lock()
	c.state = cellStopped
	c.stopErr = nil
	c.admitMu.Unlock()
	for {
		select {
		case task := <-c.queue:
			c.ref.system.recordDequeued(task)
			c.failTask(task, ErrForcedStop)
		default:
			c.ref.scope.mu.Lock()
			if entry := c.ref.scope.entries[c.ref.key]; entry != nil && entry.cell == c {
				delete(c.ref.scope.entries, c.ref.key)
			}
			c.ref.scope.mu.Unlock()
			close(c.done)
			return
		}
	}
}

func (c *actorCell) failTask(task actorTask, err error) {
	if task.result != nil {
		task.result <- taskResult{err: err}
		return
	}
	log.Error("actor accepted Tell task was discarded: %v", err)
}

func (c *actorCell) forceRemove() {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	c.admitMu.Lock()
	c.state = cellStopped
	c.stopErr = nil
	c.admitMu.Unlock()
	c.ref.scope.mu.Lock()
	if entry := c.ref.scope.entries[c.ref.key]; entry != nil && entry.cell == c {
		delete(c.ref.scope.entries, c.ref.key)
	}
	c.ref.scope.mu.Unlock()
}

func (c *actorCell) quarantine() {
	c.admitMu.Lock()
	c.state = cellQuarantined
	c.admitMu.Unlock()

	for {
		select {
		case task := <-c.queue:
			c.ref.system.recordDequeued(task)
			c.failTask(task, ErrActorQuarantined)
		default:
			c.ref.scope.mu.Lock()
			if entry := c.ref.scope.entries[c.ref.key]; entry != nil && entry.cell == c {
				entry.quarantined = true
				entry.quarantinedUntil = time.Now().Add(c.ref.system.quarantineDuration)
			}
			c.ref.scope.mu.Unlock()
			close(c.done)
			return
		}
	}
}

func (c *actorCell) isStopFailed() bool {
	c.admitMu.RLock()
	defer c.admitMu.RUnlock()
	return c.state == cellStopFailed
}

func (c *actorCell) availabilityError() error {
	c.admitMu.RLock()
	defer c.admitMu.RUnlock()
	switch c.state {
	case cellRunning:
		return nil
	case cellQuarantined:
		return ErrActorQuarantined
	default:
		return ErrActorStopped
	}
}

func (c *actorCell) retryStop(ctx context.Context) error {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()

	c.admitMu.RLock()
	state := c.state
	c.admitMu.RUnlock()
	if state == cellStopped {
		return nil
	}
	if state != cellStopFailed {
		return c.stopErr
	}

	err := c.runStopHook(ctx)
	c.stopErr = err
	if err != nil {
		return err
	}

	c.admitMu.Lock()
	c.state = cellStopped
	c.admitMu.Unlock()
	c.ref.scope.mu.Lock()
	if entry := c.ref.scope.entries[c.ref.key]; entry != nil && entry.cell == c {
		delete(c.ref.scope.entries, c.ref.key)
	}
	c.ref.scope.mu.Unlock()
	return nil
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

func errorsIsActorStopped(err error) bool {
	return errors.Is(err, ErrActorStopped)
}
