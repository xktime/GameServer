package actor

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestOrdinaryStopsPreserveQuarantinedActors(t *testing.T) {
	for _, kind := range []string{"ref", "definition", "scope", "system"} {
		t.Run(kind, func(t *testing.T) {
			system := NewActorSystem(time.Second)
			t.Cleanup(func() { _ = system.Stop(context.Background()) })
			scope, err := system.NewScope("quarantine-stop")
			if err != nil {
				t.Fatal(err)
			}
			definition, err := Define(scope, func(context.Context, string) (*stopHookTestActor, error) {
				return &stopHookTestActor{}, nil
			}, DefinitionOptions{})
			if err != nil {
				t.Fatal(err)
			}
			instance, err := definition.GetOrCreate(context.Background(), "broken")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = instance.Ref().ForceStop(context.Background()) })
			healthy, err := definition.GetOrCreate(context.Background(), "healthy")
			if err != nil {
				t.Fatal(err)
			}
			for range 3 {
				if _, err := Command(context.Background(), instance.Ref(), func(Context) (struct{}, error) {
					panic("quarantine")
				}); !errors.Is(err, ErrHandlerPanic) {
					t.Fatalf("panicking command = %v", err)
				}
			}
			target, err := definition.QuarantineTarget(context.Background(), "broken")
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "ref":
				err = instance.Ref().Stop(context.Background())
			case "definition":
				err = definition.Stop(context.Background(), "broken")
			case "scope":
				err = scope.Stop(context.Background())
			case "system":
				err = system.Stop(context.Background())
			}
			if !errors.Is(err, ErrActorQuarantined) {
				t.Fatalf("Stop = %v; want ErrActorQuarantined", err)
			}
			if got := instance.stopCalled.Load(); got != 0 {
				t.Fatalf("quarantined actor ran %d stop hooks", got)
			}
			if _, err := Query(context.Background(), instance.Ref(), func(Context) (struct{}, error) {
				return struct{}{}, nil
			}); !errors.Is(err, ErrActorQuarantined) {
				t.Fatalf("quarantined instance was not retained: %v", err)
			}
			if kind == "scope" || kind == "system" {
				if got := healthy.stopCalled.Load(); got != 1 {
					t.Fatalf("healthy actor stop hooks = %d; want 1", got)
				}
			}
			retained, err := definition.QuarantineTarget(context.Background(), "broken")
			if err != nil || retained != target {
				t.Fatalf("ordinary Stop changed quarantine identity: %v, %v", retained, err)
			}
			if err := definition.ForceStopQuarantined(context.Background(), target); err != nil {
				t.Fatalf("explicit disposal after ordinary Stop = %v", err)
			}
			if got := instance.stopCalled.Load(); got != 0 {
				t.Fatalf("explicit disposal ran %d stop hooks", got)
			}
		})
	}
}

func newQuarantineDefinition(t *testing.T, system *ActorSystem, name string) *Definition[*stopHookTestActor, string] {
	t.Helper()
	scope, err := system.NewScope(name)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := Define(scope, func(context.Context, string) (*stopHookTestActor, error) {
		return &stopHookTestActor{}, nil
	}, DefinitionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return definition
}

func quarantineTestInstance(t *testing.T, definition *Definition[*stopHookTestActor, string], id string) (*stopHookTestActor, QuarantineTarget[string]) {
	t.Helper()
	instance, err := definition.GetOrCreate(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = instance.Ref().ForceStop(context.Background()) })
	for range 3 {
		if _, err := Command(context.Background(), instance.Ref(), func(Context) (struct{}, error) {
			panic("quarantine")
		}); !errors.Is(err, ErrHandlerPanic) {
			t.Fatalf("panicking command = %v", err)
		}
	}
	target, err := definition.QuarantineTarget(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return instance, target
}

func TestQuarantineDisposalIsBoundToOneGeneration(t *testing.T) {
	system := NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	definition := newQuarantineDefinition(t, system, "generation")
	instance, target := quarantineTestInstance(t, definition, "player")
	if target.ID() != "player" || target.Generation() == 0 || target.SystemID() == "" {
		t.Fatalf("invalid observable identity: %v", target)
	}
	for range 2 {
		if err := definition.ForceStopQuarantined(context.Background(), target); err != nil {
			t.Fatalf("idempotent disposal = %v", err)
		}
	}
	if _, err := definition.Lookup(context.Background(), "player"); !errors.Is(err, ErrActorNotFound) {
		t.Fatalf("successful disposal left instance registered: %v", err)
	}
	if _, err := definition.QuarantineTarget(context.Background(), "player"); !errors.Is(err, ErrActorNotFound) {
		t.Fatalf("identity after disposal = %v", err)
	}
	replacement, err := definition.GetOrCreate(context.Background(), "player")
	if err != nil || replacement == instance {
		t.Fatalf("recreation = %p, %v", replacement, err)
	}
	if err := definition.ForceStopQuarantined(context.Background(), target); !errors.Is(err, ErrStaleQuarantineTarget) {
		t.Fatalf("old target against running replacement = %v", err)
	}
	if _, err := Query(context.Background(), replacement.Ref(), func(Context) (struct{}, error) {
		return struct{}{}, nil
	}); err != nil {
		t.Fatalf("old target affected running replacement: %v", err)
	}
	_, next := quarantineTestInstance(t, definition, "player")
	if next.SystemID() != target.SystemID() || next.Generation() <= target.Generation() {
		t.Fatalf("replacement identity = %v; original = %v", next, target)
	}
	if err := definition.ForceStopQuarantined(context.Background(), target); !errors.Is(err, ErrStaleQuarantineTarget) {
		t.Fatalf("old target against quarantined replacement = %v", err)
	}
	if current, err := definition.QuarantineTarget(context.Background(), "player"); err != nil || current != next {
		t.Fatalf("old target changed replacement quarantine: %v, %v", current, err)
	}
	if err := definition.ForceStopQuarantined(context.Background(), next); err != nil {
		t.Fatal(err)
	}
	if err := definition.ForceStopQuarantined(context.Background(), target); !errors.Is(err, ErrStaleQuarantineTarget) {
		t.Fatalf("old target after replacement was removed = %v", err)
	}
	if instance.stopCalled.Load() != 0 || replacement.stopCalled.Load() != 0 {
		t.Fatal("quarantine disposal called a domain stop hook")
	}
}

func TestQuarantineTargetsRejectOtherDefinitionsAndSystems(t *testing.T) {
	system := NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	definition := newQuarantineDefinition(t, system, "owner")
	_, target := quarantineTestInstance(t, definition, "player")
	otherScope := newQuarantineDefinition(t, system, "other")
	_, otherTarget := quarantineTestInstance(t, otherScope, "player")
	otherType, err := Define(definition.scope, func(context.Context, string) (*systemTestActor, error) {
		return &systemTestActor{}, nil
	}, DefinitionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	healthy, err := otherType.GetOrCreate(context.Background(), "player")
	if err != nil {
		t.Fatal(err)
	}
	if err := otherScope.ForceStopQuarantined(context.Background(), target); !errors.Is(err, ErrStaleQuarantineTarget) {
		t.Fatalf("cross-scope target = %v", err)
	}
	if err := otherType.ForceStopQuarantined(context.Background(), target); !errors.Is(err, ErrStaleQuarantineTarget) {
		t.Fatalf("cross-definition target = %v", err)
	}
	if current, err := otherScope.QuarantineTarget(context.Background(), "player"); err != nil || current != otherTarget {
		t.Fatalf("cross-scope instruction affected target: %v, %v", current, err)
	}
	if _, err := Query(context.Background(), healthy.Ref(), func(Context) (struct{}, error) {
		return struct{}{}, nil
	}); err != nil {
		t.Fatalf("cross-definition instruction affected healthy actor: %v", err)
	}
	if err := system.Stop(context.Background()); !errors.Is(err, ErrActorQuarantined) {
		t.Fatalf("old system Stop = %v", err)
	}
	restarted := NewActorSystem(time.Second)
	t.Cleanup(func() { _ = restarted.Stop(context.Background()) })
	restartedDefinition := newQuarantineDefinition(t, restarted, "owner")
	fresh, err := restartedDefinition.GetOrCreate(context.Background(), "player")
	if err != nil {
		t.Fatalf("new system inherited old quarantine: %v", err)
	}
	_, restartedTarget := quarantineTestInstance(t, restartedDefinition, "player")
	if target.SystemID() == restartedTarget.SystemID() || target.Generation() != restartedTarget.Generation() {
		t.Fatalf("system identities do not distinguish colliding generations: %v, %v", target, restartedTarget)
	}
	if err := restartedDefinition.ForceStopQuarantined(context.Background(), target); !errors.Is(err, ErrStaleQuarantineTarget) {
		t.Fatalf("previous-system target = %v", err)
	}
	if current, err := restartedDefinition.QuarantineTarget(context.Background(), "player"); err != nil || current != restartedTarget {
		t.Fatalf("previous-system instruction changed quarantine: %v, %v", current, err)
	}
	if fresh.stopCalled.Load() != 0 {
		t.Fatal("previous-system instruction ran a stop hook")
	}
}

func TestQuarantineObservationAndCanceledDisposalDoNotChangeInstances(t *testing.T) {
	system := NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	definition := newQuarantineDefinition(t, system, "cancellation")
	if _, err := definition.QuarantineTarget(context.Background(), "missing"); !errors.Is(err, ErrActorNotFound) {
		t.Fatalf("missing identity = %v", err)
	}
	if _, err := definition.Lookup(context.Background(), "missing"); !errors.Is(err, ErrActorNotFound) {
		t.Fatalf("observation created an instance: %v", err)
	}
	if _, err := definition.GetOrCreate(context.Background(), "healthy"); err != nil {
		t.Fatal(err)
	}
	if _, err := definition.QuarantineTarget(context.Background(), "healthy"); !errors.Is(err, ErrActorNotQuarantined) {
		t.Fatalf("running identity = %v", err)
	}
	instance, target := quarantineTestInstance(t, definition, "player")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := definition.ForceStopQuarantined(ctx, target); !errors.Is(err, context.Canceled) || errors.Is(err, ErrActorStopping) {
		t.Fatalf("canceled disposal before acceptance = %v", err)
	}
	if _, err := definition.QuarantineTarget(ctx, "player"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled observation = %v", err)
	}
	if err := definition.ForceStopQuarantined(context.Background(), QuarantineTarget[string]{}); !errors.Is(err, ErrStaleQuarantineTarget) {
		t.Fatalf("zero target = %v", err)
	}
	if current, err := definition.QuarantineTarget(context.Background(), "player"); err != nil || current != target {
		t.Fatalf("rejected disposal changed quarantine: %v, %v", current, err)
	}
	if got := instance.stopCalled.Load(); got != 0 {
		t.Fatalf("rejected disposal ran %d stop hooks", got)
	}
}

func TestConcurrentQuarantineDisposalsShareCompletion(t *testing.T) {
	system := NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	definition := newQuarantineDefinition(t, system, "concurrent")
	for range 8 {
		instance, target := quarantineTestInstance(t, definition, "player")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		const waiters = 16
		start := make(chan struct{})
		results := make(chan error, waiters)
		for range waiters {
			go func() {
				<-start
				results <- definition.ForceStopQuarantined(ctx, target)
			}()
		}
		close(start)
		for range waiters {
			select {
			case err := <-results:
				if err != nil {
					t.Fatalf("concurrent disposal = %v", err)
				}
				if _, err := definition.Lookup(context.Background(), "player"); !errors.Is(err, ErrActorNotFound) {
					t.Fatalf("disposal reported success before removal: %v", err)
				}
			case <-ctx.Done():
				t.Fatal("concurrent disposal did not complete")
			}
		}
		cancel()
		if got := instance.stopCalled.Load(); got != 0 {
			t.Fatalf("concurrent disposal ran %d stop hooks", got)
		}
	}
}

func TestAcceptedQuarantineDisposalCompletesDespiteCancellation(t *testing.T) {
	system := NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	definition := newQuarantineDefinition(t, system, "accepted-cancellation")
	instance, target := quarantineTestInstance(t, definition, "player")
	cell, err := instance.Ref().resolveCell()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- definition.ForceStopQuarantined(ctx, target) }()
	select {
	case <-cell.forceCh:
	case <-time.After(time.Second):
		t.Fatal("disposal was not accepted")
	}
	cancel()
	if err := <-result; err != nil && (!errors.Is(err, ErrActorStopping) || !errors.Is(err, context.Canceled)) {
		t.Fatalf("wait after accepted disposal = %v", err)
	}
	retryContext, cancelRetry := context.WithTimeout(context.Background(), time.Second)
	defer cancelRetry()
	if err := definition.ForceStopQuarantined(retryContext, target); err != nil {
		t.Fatalf("retry after canceled wait = %v", err)
	}
	if _, err := definition.Lookup(context.Background(), "player"); !errors.Is(err, ErrActorNotFound) {
		t.Fatalf("accepted disposal was canceled: %v", err)
	}
}

func TestQuarantineDisposalPreservesOriginalStopResult(t *testing.T) {
	system := NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	definition := newQuarantineDefinition(t, system, "original-result")
	instance, err := definition.GetOrCreate(context.Background(), "player")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = instance.Ref().ForceStop(context.Background()) })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for range 2 {
		if _, err := Command(context.Background(), instance.Ref(), func(Context) (struct{}, error) {
			panic("quarantine")
		}); !errors.Is(err, ErrHandlerPanic) {
			t.Fatalf("panicking command = %v", err)
		}
	}
	started, beginStop, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	beginStopOnce := sync.OnceFunc(func() { close(beginStop) })
	releaseOnce := sync.OnceFunc(func() { close(release) })
	t.Cleanup(beginStopOnce)
	t.Cleanup(releaseOnce)
	stopping := make(chan StopResult, 1)
	panicked := make(chan error, 1)
	go func() {
		_, err := Command(ctx, instance.Ref(), func(execution Context) (struct{}, error) {
			close(started)
			<-beginStop
			result, err := execution.StopSelf(context.Background())
			if err != nil {
				return struct{}{}, err
			}
			stopping <- result
			<-release
			panic("quarantine during stop")
		})
		panicked <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("handler did not start")
	}
	// 排队任务的拒绝发生在隔离状态公布之后，处置不能接管原普通停止结果。
	for range 256 {
		if err := TryTell(instance.Ref(), func(Context) error {
			t.Error("queued task ran after quarantine")
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	beginStopOnce()
	var original StopResult
	select {
	case original = <-stopping:
	case <-ctx.Done():
		t.Fatal("handler did not request its own stop")
	}
	releaseOnce()
	var target QuarantineTarget[string]
	for {
		target, err = definition.QuarantineTarget(ctx, "player")
		if err == nil {
			break
		}
		if !errors.Is(err, ErrActorNotQuarantined) {
			t.Fatalf("observe quarantine during queue rejection = %v", err)
		}
		runtime.Gosched()
	}
	if err := definition.ForceStopQuarantined(ctx, target); err != nil {
		t.Fatal(err)
	}
	if err := <-panicked; !errors.Is(err, ErrHandlerPanic) {
		t.Fatalf("panicking command = %v", err)
	}
	if err := original.Wait(ctx); !errors.Is(err, ErrActorQuarantined) {
		t.Fatalf("disposal changed the original stop result: %v", err)
	}
	if got := instance.stopCalled.Load(); got != 0 {
		t.Fatalf("quarantined stop attempt ran %d hooks", got)
	}
}
