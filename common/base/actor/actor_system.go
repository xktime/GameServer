package actor

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	initializationFailureBackoff = 100 * time.Millisecond
	defaultQueueCapacity         = 256
	defaultPanicThreshold        = 3
)

// ActorSystem 管理 Actor 作用域及其共享生命周期。
type ActorSystem struct {
	instanceID     string
	ctx            context.Context
	cancel         context.CancelFunc
	timeout        time.Duration
	panicThreshold int

	mu      sync.Mutex
	scopes  map[string]*Scope
	stopped bool
	metrics actorMetrics
}

// NewActorSystem 创建管理 Actor 运行时及其生命周期的实例。
func NewActorSystem(timeout time.Duration) *ActorSystem {
	ctx, cancel := context.WithCancel(context.Background())
	return &ActorSystem{
		instanceID:     uuid.NewString(),
		ctx:            ctx,
		cancel:         cancel,
		timeout:        timeout,
		panicThreshold: defaultPanicThreshold,
		scopes:         make(map[string]*Scope),
	}
}

// NewScope 创建名称唯一的领域作用域。
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
		definitions: make(map[reflect.Type]struct{}),
		entries:     make(map[actorKey]*actorEntry),
		generations: make(map[actorKey]uint64),
	}
	s.scopes[name] = scope
	return scope, nil
}

// Stop 先有序关闭所有作用域，再取消运行时上下文。
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
