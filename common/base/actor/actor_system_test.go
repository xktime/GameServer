package actor

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type systemTestActor struct {
	BaseActor
	id    string
	ready bool
}

type stopHookTestActor struct {
	BaseActor
	applied    atomic.Int32
	stoppedAt  atomic.Int32
	stopCalled atomic.Int32
	failStops  atomic.Int32
	panicStops atomic.Int32
	stopError  error
}

type blockingStopTestActor struct {
	BaseActor
	started chan struct{}
	release chan struct{}
}

func (a *blockingStopTestActor) OnStop(context.Context) error {
	close(a.started)
	<-a.release
	return nil
}

func (a *stopHookTestActor) OnStop(context.Context) error {
	call := a.stopCalled.Add(1)
	a.stoppedAt.Store(a.applied.Load())
	if call <= a.panicStops.Load() {
		panic("stop hook panic")
	}
	if call <= a.failStops.Load() {
		return a.stopError
	}
	return nil
}

func TestActorRefStopRecoversDomainHookPanicAndAllowsRetry(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	definition, err := Define(scope, Test1, func(context.Context, string) (*stopHookTestActor, error) {
		instance := &stopHookTestActor{}
		instance.panicStops.Store(1)
		return instance, nil
	})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}
	instance, err := definition.GetOrCreate(context.Background(), "panic-stop")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}

	stopContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := instance.Ref().Stop(stopContext); err == nil {
		t.Fatal("first Stop succeeded; want recovered stop-hook panic")
	}
	if err := instance.Ref().Stop(stopContext); err != nil {
		t.Fatalf("retry Stop: %v", err)
	}
	if got := instance.stopCalled.Load(); got != 2 {
		t.Fatalf("stop hook calls = %d; want 2", got)
	}
}

func TestDefinitionGetOrCreateCoalescesConcurrentInitialization(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}

	var initializationCount atomic.Int32
	initializationStarted := make(chan struct{}, 1)
	releaseInitialization := make(chan struct{})
	definition, err := Define(scope, Test1, func(ctx context.Context, id string) (*systemTestActor, error) {
		initializationCount.Add(1)
		initializationStarted <- struct{}{}
		select {
		case <-releaseInitialization:
			return &systemTestActor{id: id, ready: true}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}

	const callers = 32
	actors := make([]*systemTestActor, callers)
	errors := make([]error, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := range callers {
		go func(index int) {
			defer wg.Done()
			actors[index], errors[index] = definition.GetOrCreate(context.Background(), "shared")
		}(i)
	}

	select {
	case <-initializationStarted:
	case <-time.After(time.Second):
		t.Fatal("initialization did not start")
	}
	close(releaseInitialization)
	wg.Wait()

	if got := initializationCount.Load(); got != 1 {
		t.Fatalf("factory ran %d times; want 1", got)
	}
	first := actors[0]
	if first == nil || !first.ready {
		t.Fatalf("first caller received an actor before it was ready: %#v", first)
	}
	for i := range callers {
		if errors[i] != nil {
			t.Fatalf("caller %d: %v", i, errors[i])
		}
		if actors[i] != first {
			t.Fatalf("caller %d received %p; want shared actor %p", i, actors[i], first)
		}
	}
}

func TestDefinitionRejectsSecondFactoryForTypeInSameScope(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	if _, err := Define(scope, Test1, func(context.Context, string) (*systemTestActor, error) {
		return &systemTestActor{}, nil
	}); err != nil {
		t.Fatalf("define first factory: %v", err)
	}
	if _, err := Define(scope, Test2, func(context.Context, int64) (*systemTestActor, error) {
		return &systemTestActor{}, nil
	}); err == nil {
		t.Fatal("second factory for one actor type succeeded")
	}
}

func TestDefinitionLookupDoesNotCreateMissingActor(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}

	var initializationCount atomic.Int32
	definition, err := Define(scope, Test1, func(context.Context, string) (*systemTestActor, error) {
		initializationCount.Add(1)
		return &systemTestActor{}, nil
	})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}

	if _, err := definition.Lookup(context.Background(), "missing"); !errors.Is(err, ErrActorStopped) {
		t.Fatalf("lookup missing actor: got %v, want %v", err, ErrActorStopped)
	}
	if got := initializationCount.Load(); got != 0 {
		t.Fatalf("lookup initialized %d actors; want 0", got)
	}

	created, err := definition.GetOrCreate(context.Background(), "created")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}
	found, err := definition.Lookup(context.Background(), "created")
	if err != nil {
		t.Fatalf("lookup created actor: %v", err)
	}
	if found != created {
		t.Fatalf("lookup returned %p; want %p", found, created)
	}

	if err := created.Ref().Stop(context.Background()); err != nil {
		t.Fatalf("stop actor: %v", err)
	}
	if _, err := definition.Lookup(context.Background(), "created"); !errors.Is(err, ErrActorStopped) {
		t.Fatalf("lookup stopped actor: got %v, want %v", err, ErrActorStopped)
	}
}

func TestDefinitionLookupRejectsActorWhileItIsStopping(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	definition, err := Define(scope, Test1, func(context.Context, string) (*blockingStopTestActor, error) {
		return &blockingStopTestActor{started: make(chan struct{}), release: make(chan struct{})}, nil
	})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}
	instance, err := definition.GetOrCreate(context.Background(), "stopping")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}

	stopResult := make(chan error, 1)
	go func() {
		stopResult <- instance.Ref().Stop(context.Background())
	}()
	<-instance.started
	if _, err := definition.Lookup(context.Background(), "stopping"); !errors.Is(err, ErrActorStopped) {
		t.Fatalf("lookup stopping actor: got %v, want %v", err, ErrActorStopped)
	}
	if _, err := definition.GetOrCreate(context.Background(), "stopping"); !errors.Is(err, ErrActorStopped) {
		t.Fatalf("get stopping actor: got %v, want %v", err, ErrActorStopped)
	}
	close(instance.release)
	if err := <-stopResult; err != nil {
		t.Fatalf("stop actor: %v", err)
	}
}

func TestCallSerializesTasksForOneActor(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	definition, err := Define(scope, Test1, func(context.Context, string) (*systemTestActor, error) {
		return &systemTestActor{}, nil
	})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}
	instance, err := definition.GetOrCreate(context.Background(), "serial")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}

	const calls = 32
	var active atomic.Int32
	var maximumActive atomic.Int32
	var executed atomic.Int32
	errors := make(chan error, calls)
	var wg sync.WaitGroup
	wg.Add(calls)
	for range calls {
		go func() {
			defer wg.Done()
			_, callErr := Call(context.Background(), instance.Ref(), func(Context) (int32, error) {
				current := active.Add(1)
				for {
					maximum := maximumActive.Load()
					if current <= maximum || maximumActive.CompareAndSwap(maximum, current) {
						break
					}
				}
				time.Sleep(time.Millisecond)
				active.Add(-1)
				return executed.Add(1), nil
			})
			if callErr != nil {
				errors <- callErr
			}
		}()
	}
	wg.Wait()
	close(errors)
	for callErr := range errors {
		t.Errorf("call: %v", callErr)
	}

	if got := executed.Load(); got != calls {
		t.Fatalf("executed %d tasks; want %d", got, calls)
	}
	if got := maximumActive.Load(); got != 1 {
		t.Fatalf("maximum concurrent tasks = %d; want 1", got)
	}
}

func TestTellReturnsAfterTaskIsAccepted(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	definition, err := Define(scope, Test1, func(context.Context, string) (*systemTestActor, error) {
		return &systemTestActor{}, nil
	})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}
	instance, err := definition.GetOrCreate(context.Background(), "tell")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	completed := make(chan struct{})
	err = Tell(context.Background(), instance.Ref(), func(Context) error {
		close(started)
		<-release
		close(completed)
		return nil
	})
	if err != nil {
		t.Fatalf("tell: %v", err)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("accepted task did not start")
	}
	select {
	case <-completed:
		t.Fatal("Tell waited for task completion")
	default:
	}
	close(release)
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("accepted task did not complete")
	}
}

func TestTellExecutionCanCallAnotherActorAfterAdmissionReturns(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	definition, err := Define(scope, Test1, func(context.Context, string) (*systemTestActor, error) {
		return &systemTestActor{}, nil
	})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}
	first, err := definition.GetOrCreate(context.Background(), "first")
	if err != nil {
		t.Fatalf("create first actor: %v", err)
	}
	second, err := definition.GetOrCreate(context.Background(), "second")
	if err != nil {
		t.Fatalf("create second actor: %v", err)
	}

	release := make(chan struct{})
	completed := make(chan error, 1)
	if err := Tell(context.Background(), first.Ref(), func(execution Context) error {
		<-release
		_, err := Call(execution, second.Ref(), func(Context) (struct{}, error) {
			return struct{}{}, nil
		})
		completed <- err
		return err
	}); err != nil {
		t.Fatalf("tell: %v", err)
	}
	close(release)
	select {
	case err := <-completed:
		if err != nil {
			t.Fatalf("nested Call after Tell admission: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Tell task did not complete")
	}
}

func TestTellStartsANewSynchronousCallChain(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	definition, err := Define(scope, Test1, func(context.Context, string) (*systemTestActor, error) {
		return &systemTestActor{}, nil
	})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}
	first, err := definition.GetOrCreate(context.Background(), "first")
	if err != nil {
		t.Fatalf("create first actor: %v", err)
	}
	second, err := definition.GetOrCreate(context.Background(), "second")
	if err != nil {
		t.Fatalf("create second actor: %v", err)
	}

	nestedResult := make(chan error, 1)
	_, err = Call(context.Background(), first.Ref(), func(execution Context) (struct{}, error) {
		return struct{}{}, Tell(execution, second.Ref(), func(secondExecution Context) error {
			_, err := Call(secondExecution, first.Ref(), func(Context) (struct{}, error) {
				return struct{}{}, nil
			})
			nestedResult <- err
			return err
		})
	})
	if err != nil {
		t.Fatalf("outer Call: %v", err)
	}
	select {
	case err := <-nestedResult:
		if err != nil {
			t.Fatalf("Call after asynchronous Tell: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Tell task did not complete")
	}
}

func TestCallRejectsACycleStartedByTell(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	definition, err := Define(scope, Test1, func(context.Context, string) (*systemTestActor, error) {
		return &systemTestActor{}, nil
	})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}
	first, err := definition.GetOrCreate(context.Background(), "first")
	if err != nil {
		t.Fatalf("create first actor: %v", err)
	}
	second, err := definition.GetOrCreate(context.Background(), "second")
	if err != nil {
		t.Fatalf("create second actor: %v", err)
	}

	completed := make(chan error, 1)
	if err := Tell(context.Background(), first.Ref(), func(firstContext Context) error {
		_, err := Call(firstContext, second.Ref(), func(secondContext Context) (struct{}, error) {
			return Call(secondContext, first.Ref(), func(Context) (struct{}, error) {
				return struct{}{}, nil
			})
		})
		completed <- err
		return err
	}); err != nil {
		t.Fatalf("tell: %v", err)
	}

	select {
	case err := <-completed:
		if !errors.Is(err, ErrCallCycle) {
			t.Fatalf("Call error = %v; want ErrCallCycle", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Tell task did not complete")
	}
}

func TestTryTellReportsQueueFullAndRunsEveryAcceptedTask(t *testing.T) {
	system := NewActorSystem(5 * time.Second)
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	definition, err := Define(scope, Test1, func(context.Context, string) (*systemTestActor, error) {
		return &systemTestActor{}, nil
	})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}
	instance, err := definition.GetOrCreate(context.Background(), "full")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	if err := Tell(context.Background(), instance.Ref(), func(Context) error {
		close(started)
		<-release
		return nil
	}); err != nil {
		t.Fatalf("occupy actor: %v", err)
	}
	<-started

	var executed atomic.Int32
	accepted := 0
	for {
		err := TryTell(instance.Ref(), func(Context) error {
			executed.Add(1)
			return nil
		})
		if errors.Is(err, ErrQueueFull) {
			break
		}
		if err != nil {
			t.Fatalf("try tell %d: %v", accepted, err)
		}
		accepted++
		if accepted > 20000 {
			t.Fatal("TryTell did not report a bounded queue")
		}
	}
	if accepted == 0 {
		t.Fatal("queue accepted no tasks")
	}

	close(release)
	if _, err := Call(context.Background(), instance.Ref(), func(Context) (struct{}, error) {
		return struct{}{}, nil
	}); err != nil {
		t.Fatalf("wait for accepted tasks: %v", err)
	}
	if got := int(executed.Load()); got != accepted {
		t.Fatalf("executed %d accepted tasks; want %d", got, accepted)
	}
}

func TestCallReportsUnknownOutcomeAfterAcceptedTaskTimesOut(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	definition, err := Define(scope, Test1, func(context.Context, string) (*systemTestActor, error) {
		return &systemTestActor{}, nil
	})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}
	instance, err := definition.GetOrCreate(context.Background(), "timeout")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	var effects atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = Call(ctx, instance.Ref(), func(Context) (struct{}, error) {
		close(started)
		<-release
		effects.Add(1)
		return struct{}{}, nil
	})
	if !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("Call error = %v; want ErrOutcomeUnknown", err)
	}
	<-started
	close(release)
	if _, err := Call(context.Background(), instance.Ref(), func(Context) (struct{}, error) {
		return struct{}{}, nil
	}); err != nil {
		t.Fatalf("wait for timed-out task: %v", err)
	}
	if got := effects.Load(); got != 1 {
		t.Fatalf("timed-out task applied %d effects; want 1", got)
	}
}

func TestCallToCurrentActorExecutesInsideOuterMessage(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	definition, err := Define(scope, Test1, func(context.Context, string) (*systemTestActor, error) {
		return &systemTestActor{}, nil
	})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}
	instance, err := definition.GetOrCreate(context.Background(), "self")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}

	order := make([]string, 0, 3)
	result, err := Call(context.Background(), instance.Ref(), func(execution Context) (string, error) {
		order = append(order, "outer-before")
		inner, innerErr := Call(execution, instance.Ref(), func(Context) (string, error) {
			order = append(order, "inner")
			return "inner-result", nil
		})
		order = append(order, "outer-after")
		return inner, innerErr
	})
	if err != nil {
		t.Fatalf("self Call: %v", err)
	}
	if result != "inner-result" {
		t.Fatalf("result = %q; want inner-result", result)
	}
	want := []string{"outer-before", "inner", "outer-after"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("execution order = %v; want %v", order, want)
		}
	}
}

func TestCallRejectsAnActorCycle(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	definition, err := Define(scope, Test1, func(context.Context, string) (*systemTestActor, error) {
		return &systemTestActor{}, nil
	})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}
	first, err := definition.GetOrCreate(context.Background(), "first")
	if err != nil {
		t.Fatalf("create first actor: %v", err)
	}
	second, err := definition.GetOrCreate(context.Background(), "second")
	if err != nil {
		t.Fatalf("create second actor: %v", err)
	}

	_, err = Call(context.Background(), first.Ref(), func(firstContext Context) (struct{}, error) {
		return Call(firstContext, second.Ref(), func(secondContext Context) (struct{}, error) {
			return Call(secondContext, first.Ref(), func(Context) (struct{}, error) {
				return struct{}{}, nil
			})
		})
	})
	if !errors.Is(err, ErrCallCycle) {
		t.Fatalf("Call error = %v; want ErrCallCycle", err)
	}
}

func TestGetOrCreateRetriesAfterInitializationFailureBackoff(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	initializationFailure := errors.New("database unavailable")
	var attempts atomic.Int32
	definition, err := Define(scope, Test1, func(context.Context, string) (*systemTestActor, error) {
		if attempts.Add(1) == 1 {
			return nil, initializationFailure
		}
		return &systemTestActor{ready: true}, nil
	})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}

	if _, err := definition.GetOrCreate(context.Background(), "retry"); !errors.Is(err, initializationFailure) {
		t.Fatalf("first initialization error = %v; want %v", err, initializationFailure)
	}
	if _, err := definition.Lookup(context.Background(), "retry"); !errors.Is(err, initializationFailure) {
		t.Fatalf("lookup initialization error = %v; want %v", err, initializationFailure)
	}
	if _, err := definition.GetOrCreate(context.Background(), "retry"); !errors.Is(err, initializationFailure) {
		t.Fatalf("cached initialization error = %v; want %v", err, initializationFailure)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("initialization attempts during backoff = %d; want 1", got)
	}

	time.Sleep(initializationFailureBackoff + 10*time.Millisecond)
	instance, err := definition.GetOrCreate(context.Background(), "retry")
	if err != nil {
		t.Fatalf("retry initialization: %v", err)
	}
	if instance == nil || !instance.ready {
		t.Fatalf("retry returned unready actor: %#v", instance)
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("initialization attempts = %d; want 2", got)
	}
}

func TestActorRefStopRejectsNewTasksAndDrainsAcceptedTasks(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	definition, err := Define(scope, Test1, func(context.Context, string) (*systemTestActor, error) {
		return &systemTestActor{}, nil
	})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}
	instance, err := definition.GetOrCreate(context.Background(), "stop")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	if err := Tell(context.Background(), instance.Ref(), func(Context) error {
		close(started)
		<-release
		return nil
	}); err != nil {
		t.Fatalf("tell blocking task: %v", err)
	}
	<-started
	var drained atomic.Bool
	if err := Tell(context.Background(), instance.Ref(), func(Context) error {
		drained.Store(true)
		return nil
	}); err != nil {
		t.Fatalf("tell queued task: %v", err)
	}

	stopResult := make(chan error, 1)
	go func() {
		stopResult <- instance.Ref().Stop(context.Background())
	}()
	deadline := time.Now().Add(time.Second)
	for {
		err := TryTell(instance.Ref(), func(Context) error { return nil })
		if errors.Is(err, ErrActorStopped) {
			break
		}
		if err != nil && !errors.Is(err, ErrQueueFull) {
			t.Fatalf("probe stopped actor: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatal("actor did not stop accepting tasks")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case err := <-stopResult:
		t.Fatalf("Stop returned before accepted tasks drained: %v", err)
	default:
	}

	close(release)
	select {
	case err := <-stopResult:
		if err != nil {
			t.Fatalf("Stop: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Stop did not finish")
	}
	if !drained.Load() {
		t.Fatal("accepted task was not drained")
	}
	if _, err := Call(context.Background(), instance.Ref(), func(Context) (struct{}, error) {
		return struct{}{}, nil
	}); !errors.Is(err, ErrActorStopped) {
		t.Fatalf("Call after Stop error = %v; want ErrActorStopped", err)
	}
}

func TestActorRefStopRunsDomainHookAfterDrainingTasks(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	definition, err := Define(scope, Test1, func(context.Context, string) (*stopHookTestActor, error) {
		return &stopHookTestActor{}, nil
	})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}
	instance, err := definition.GetOrCreate(context.Background(), "persist")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}
	for range 3 {
		if err := Tell(context.Background(), instance.Ref(), func(Context) error {
			instance.applied.Add(1)
			return nil
		}); err != nil {
			t.Fatalf("Tell: %v", err)
		}
	}

	if err := instance.Ref().Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := instance.stopCalled.Load(); got != 1 {
		t.Fatalf("stop hook calls = %d; want 1", got)
	}
	if got := instance.stoppedAt.Load(); got != 3 {
		t.Fatalf("stop hook observed %d applied tasks; want 3", got)
	}
}

func TestActorRefStopKeepsFailedActorIsolatedUntilPersistenceRetrySucceeds(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	persistError := errors.New("persist failed")
	definition, err := Define(scope, Test1, func(context.Context, string) (*stopHookTestActor, error) {
		instance := &stopHookTestActor{stopError: persistError}
		instance.failStops.Store(1)
		return instance, nil
	})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}
	instance, err := definition.GetOrCreate(context.Background(), "persist-retry")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}

	if err := instance.Ref().Stop(context.Background()); !errors.Is(err, persistError) {
		t.Fatalf("first Stop error = %v; want %v", err, persistError)
	}
	if _, err := Call(context.Background(), instance.Ref(), func(Context) (struct{}, error) {
		return struct{}{}, nil
	}); !errors.Is(err, ErrActorStopped) {
		t.Fatalf("Call while persistence is failed = %v; want ErrActorStopped", err)
	}
	if err := instance.Ref().Stop(context.Background()); err != nil {
		t.Fatalf("retry Stop: %v", err)
	}
	if got := instance.stopCalled.Load(); got != 2 {
		t.Fatalf("stop hook calls = %d; want 2", got)
	}
}

func TestActorContinuesAfterHandlerPanic(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	definition, err := Define(scope, Test1, func(context.Context, string) (*systemTestActor, error) {
		return &systemTestActor{}, nil
	})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}
	instance, err := definition.GetOrCreate(context.Background(), "panic")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}

	if _, err := Call(context.Background(), instance.Ref(), func(Context) (struct{}, error) {
		panic("broken handler")
	}); !errors.Is(err, ErrHandlerPanic) {
		t.Fatalf("panic Call error = %v; want ErrHandlerPanic", err)
	}
	result, err := Call(context.Background(), instance.Ref(), func(Context) (string, error) {
		return "still-running", nil
	})
	if err != nil {
		t.Fatalf("Call after panic: %v", err)
	}
	if result != "still-running" {
		t.Fatalf("Call after panic = %q; want still-running", result)
	}
}

func TestActorIsQuarantinedAfterThreeConsecutivePanicsAndRebuildsAfterCooldown(t *testing.T) {
	system := NewActorSystem(time.Second)
	system.quarantineDuration = 20 * time.Millisecond
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	var creations atomic.Int32
	definition, err := Define(scope, Test1, func(context.Context, string) (*systemTestActor, error) {
		creations.Add(1)
		return &systemTestActor{}, nil
	})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}
	instance, err := definition.GetOrCreate(context.Background(), "quarantine")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}
	oldRef := instance.Ref()

	for attempt := 1; attempt <= 3; attempt++ {
		if _, err := Call(context.Background(), oldRef, func(Context) (struct{}, error) {
			panic("deterministic failure")
		}); !errors.Is(err, ErrHandlerPanic) {
			t.Fatalf("panic %d error = %v; want ErrHandlerPanic", attempt, err)
		}
	}
	if _, err := Call(context.Background(), oldRef, func(Context) (struct{}, error) {
		return struct{}{}, nil
	}); !errors.Is(err, ErrActorQuarantined) {
		t.Fatalf("Call during quarantine = %v; want ErrActorQuarantined", err)
	}
	if _, err := definition.GetOrCreate(context.Background(), "quarantine"); !errors.Is(err, ErrActorQuarantined) {
		t.Fatalf("GetOrCreate during quarantine = %v; want ErrActorQuarantined", err)
	}

	time.Sleep(system.quarantineDuration + 10*time.Millisecond)
	rebuilt, err := definition.GetOrCreate(context.Background(), "quarantine")
	if err != nil {
		t.Fatalf("rebuild actor: %v", err)
	}
	if rebuilt == instance {
		t.Fatal("cooldown reused quarantined actor")
	}
	if got := creations.Load(); got != 2 {
		t.Fatalf("actor creations = %d; want 2", got)
	}
	if _, err := Call(context.Background(), oldRef, func(Context) (struct{}, error) {
		return struct{}{}, nil
	}); !errors.Is(err, ErrStaleActorRef) {
		t.Fatalf("old reference error = %v; want ErrStaleActorRef", err)
	}
}

func TestScopeStopOnlyStopsActorsOwnedByThatScope(t *testing.T) {
	system := NewActorSystem(time.Second)
	firstScope, err := system.NewScope("first")
	if err != nil {
		t.Fatalf("create first scope: %v", err)
	}
	secondScope, err := system.NewScope("second")
	if err != nil {
		t.Fatalf("create second scope: %v", err)
	}
	firstDefinition, err := Define(firstScope, Test1, func(context.Context, string) (*systemTestActor, error) {
		return &systemTestActor{}, nil
	})
	if err != nil {
		t.Fatalf("define first actor: %v", err)
	}
	secondDefinition, err := Define(secondScope, Test1, func(context.Context, string) (*systemTestActor, error) {
		return &systemTestActor{}, nil
	})
	if err != nil {
		t.Fatalf("define second actor: %v", err)
	}
	first, err := firstDefinition.GetOrCreate(context.Background(), "one")
	if err != nil {
		t.Fatalf("create first actor: %v", err)
	}
	second, err := secondDefinition.GetOrCreate(context.Background(), "two")
	if err != nil {
		t.Fatalf("create second actor: %v", err)
	}

	if err := firstScope.Stop(context.Background()); err != nil {
		t.Fatalf("stop first scope: %v", err)
	}
	if _, err := Call(context.Background(), first.Ref(), func(Context) (struct{}, error) {
		return struct{}{}, nil
	}); !errors.Is(err, ErrActorStopped) {
		t.Fatalf("first actor error = %v; want ErrActorStopped", err)
	}
	if _, err := firstDefinition.GetOrCreate(context.Background(), "new"); !errors.Is(err, ErrScopeStopped) {
		t.Fatalf("GetOrCreate in stopped scope = %v; want ErrScopeStopped", err)
	}
	result, err := Call(context.Background(), second.Ref(), func(Context) (string, error) {
		return "running", nil
	})
	if err != nil {
		t.Fatalf("second actor Call: %v", err)
	}
	if result != "running" {
		t.Fatalf("second actor result = %q; want running", result)
	}
}

func TestActorSystemStopClosesAllScopesOnce(t *testing.T) {
	system := NewActorSystem(time.Second)
	refs := make([]ActorRef, 0, 2)
	for _, name := range []string{"first", "second"} {
		scope, err := system.NewScope(name)
		if err != nil {
			t.Fatalf("create %s scope: %v", name, err)
		}
		definition, err := Define(scope, Test1, func(context.Context, string) (*systemTestActor, error) {
			return &systemTestActor{}, nil
		})
		if err != nil {
			t.Fatalf("define %s actor: %v", name, err)
		}
		instance, err := definition.GetOrCreate(context.Background(), name)
		if err != nil {
			t.Fatalf("create %s actor: %v", name, err)
		}
		refs = append(refs, instance.Ref())
	}

	if err := system.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := system.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	for i, ref := range refs {
		if _, err := Call(context.Background(), ref, func(Context) (struct{}, error) {
			return struct{}{}, nil
		}); !errors.Is(err, ErrActorStopped) {
			t.Fatalf("actor %d error = %v; want ErrActorStopped", i, err)
		}
	}
	if _, err := system.NewScope("late"); !errors.Is(err, ErrSystemStopped) {
		t.Fatalf("NewScope after Stop = %v; want ErrSystemStopped", err)
	}
}

func TestScopeStopCancelsSharedInitialization(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	started := make(chan struct{})
	definition, err := Define(scope, Test1, func(ctx context.Context, _ string) (*systemTestActor, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}
	initializationResult := make(chan error, 1)
	go func() {
		_, err := definition.GetOrCreate(context.Background(), "initializing")
		initializationResult <- err
	}()
	<-started

	stopContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := scope.Stop(stopContext); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case err := <-initializationResult:
		if !errors.Is(err, ErrScopeStopped) {
			t.Fatalf("initialization error = %v; want ErrScopeStopped", err)
		}
	case <-time.After(time.Second):
		t.Fatal("scope-owned initialization was not canceled")
	}
}

func TestForceStopFailsQueuedCallsAndSkipsDomainStopHook(t *testing.T) {
	system := NewActorSystem(time.Second)
	system.queueCapacity = 1
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	definition, err := Define(scope, Test1, func(context.Context, string) (*stopHookTestActor, error) {
		return &stopHookTestActor{}, nil
	})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}
	instance, err := definition.GetOrCreate(context.Background(), "force")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	if err := Tell(context.Background(), instance.Ref(), func(Context) error {
		close(started)
		<-release
		return nil
	}); err != nil {
		t.Fatalf("occupy actor: %v", err)
	}
	<-started
	queuedCall := make(chan error, 1)
	go func() {
		_, err := Call(context.Background(), instance.Ref(), func(Context) (struct{}, error) {
			return struct{}{}, nil
		})
		queuedCall <- err
	}()
	cell, err := instance.Ref().resolveCell()
	if err != nil {
		t.Fatalf("resolve actor cell: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for len(cell.queue) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("Call was not accepted into the queue")
		}
		time.Sleep(time.Millisecond)
	}

	forceResult := make(chan error, 1)
	go func() {
		forceResult <- instance.Ref().ForceStop(context.Background())
	}()
	deadline = time.Now().Add(time.Second)
	for {
		err := TryTell(instance.Ref(), func(Context) error { return nil })
		if errors.Is(err, ErrActorStopped) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("ForceStop did not reject new tasks")
		}
		time.Sleep(time.Millisecond)
	}
	close(release)

	select {
	case err := <-queuedCall:
		if !errors.Is(err, ErrForcedStop) {
			t.Fatalf("queued Call error = %v; want ErrForcedStop", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued Call was not failed")
	}
	select {
	case err := <-forceResult:
		if err != nil {
			t.Fatalf("ForceStop: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ForceStop did not finish")
	}
	if got := instance.stopCalled.Load(); got != 0 {
		t.Fatalf("domain stop hook calls = %d; want 0", got)
	}
}

func TestActorCanRequestOwnStopWithoutWaitingOnItself(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	definition, err := Define(scope, Test1, func(context.Context, string) (*systemTestActor, error) {
		return &systemTestActor{}, nil
	})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}
	instance, err := definition.GetOrCreate(context.Background(), "self-stop")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}

	deferredTask := make(chan struct{})
	_, err = Call(context.Background(), instance.Ref(), func(execution Context) (struct{}, error) {
		if err := Tell(execution, instance.Ref(), func(Context) error {
			close(deferredTask)
			return nil
		}); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, instance.Ref().Stop(execution)
	})
	if err != nil {
		t.Fatalf("self Stop: %v", err)
	}
	select {
	case <-deferredTask:
	case <-time.After(time.Second):
		t.Fatal("accepted self Tell was not drained")
	}
	if _, err := Call(context.Background(), instance.Ref(), func(Context) (struct{}, error) {
		return struct{}{}, nil
	}); !errors.Is(err, ErrActorStopped) {
		t.Fatalf("Call after self Stop = %v; want ErrActorStopped", err)
	}
}

func TestActorSystemMetricsTrackAdmissionQueueWaitAndPanics(t *testing.T) {
	system := NewActorSystem(time.Second)
	system.queueCapacity = 1
	scope, err := system.NewScope("test")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	definition, err := Define(scope, Test1, func(context.Context, string) (*systemTestActor, error) {
		return &systemTestActor{}, nil
	})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}
	instance, err := definition.GetOrCreate(context.Background(), "metrics")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	if err := Tell(context.Background(), instance.Ref(), func(Context) error {
		close(started)
		<-release
		return nil
	}); err != nil {
		t.Fatalf("occupy actor: %v", err)
	}
	<-started
	queuedDone := make(chan struct{})
	if err := Tell(context.Background(), instance.Ref(), func(Context) error {
		close(queuedDone)
		return nil
	}); err != nil {
		t.Fatalf("queue task: %v", err)
	}
	if err := TryTell(instance.Ref(), func(Context) error { return nil }); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("TryTell full queue = %v; want ErrQueueFull", err)
	}
	if got := system.Metrics().QueueDepth; got != 1 {
		t.Fatalf("queue depth = %d; want 1", got)
	}
	time.Sleep(10 * time.Millisecond)
	close(release)
	<-queuedDone

	_, err = Call(context.Background(), instance.Ref(), func(Context) (struct{}, error) {
		panic("metrics panic")
	})
	if !errors.Is(err, ErrHandlerPanic) {
		t.Fatalf("panic Call = %v; want ErrHandlerPanic", err)
	}
	metrics := system.Metrics()
	if metrics.AcceptedTasks != 3 {
		t.Fatalf("accepted tasks = %d; want 3", metrics.AcceptedTasks)
	}
	if metrics.RejectedTasks != 1 {
		t.Fatalf("rejected tasks = %d; want 1", metrics.RejectedTasks)
	}
	if metrics.HandlerPanics != 1 {
		t.Fatalf("handler panics = %d; want 1", metrics.HandlerPanics)
	}
	if metrics.QueueDepth != 0 {
		t.Fatalf("final queue depth = %d; want 0", metrics.QueueDepth)
	}
	if metrics.QueueWaitTotal <= 0 || metrics.QueueWaitMax <= 0 {
		t.Fatalf("queue waits = total %v, max %v; want positive durations", metrics.QueueWaitTotal, metrics.QueueWaitMax)
	}
}
