package actor

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type persistenceTestActor struct {
	BaseActor
	save func(context.Context) error
}

func (a *persistenceTestActor) OnStop(ctx context.Context) error { return a.save(ctx) }

func TestConcurrentStopsShareAttemptAndRetryPreservesOriginalResult(t *testing.T) {
	system := NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	scope, _ := system.NewScope("stop-attempt")
	started, release := make(chan struct{}), make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseOnce)
	failure := errors.New("storage unavailable")
	var saves atomic.Int32
	definition, err := Define(scope, func(context.Context, string) (*persistenceTestActor, error) {
		return &persistenceTestActor{save: func(context.Context) error {
			if saves.Add(1) == 1 {
				close(started)
				<-release
				return failure
			}
			return nil
		}}, nil
	}, DefinitionOptions{QueueCapacity: 8})
	if err != nil {
		t.Fatal(err)
	}
	instance, _ := definition.GetOrCreate(context.Background(), "player")
	result, err := Command(context.Background(), instance.Ref(), func(execution Context) (StopResult, error) {
		return execution.StopSelf(context.Background())
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	const waiters = 8
	joined := make(chan error, waiters)
	for range waiters {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			joined <- definition.Stop(ctx, "player")
		}()
	}
	for range waiters {
		if err := <-joined; !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, ErrActorStopping) {
			t.Fatalf("concurrent Stop = %v", err)
		}
	}
	if saves.Load() != 1 {
		t.Fatalf("concurrent stops ran %d saves", saves.Load())
	}
	releaseOnce()
	if err := result.Wait(context.Background()); !errors.Is(err, ErrStopFailed) || !errors.Is(err, failure) {
		t.Fatalf("original result = %v", err)
	}
	if _, err := definition.GetOrCreate(context.Background(), "player"); !errors.Is(err, ErrStopFailed) {
		t.Fatalf("replacement after failed persistence = %v", err)
	}
	if err := definition.Stop(context.Background(), "player"); err != nil {
		t.Fatalf("retry Stop = %v", err)
	}
	if saves.Load() != 2 {
		t.Fatalf("retry ran %d saves; want 2", saves.Load())
	}
	if err := result.Wait(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("retry changed the original stop result: %v", err)
	}
	if _, err := definition.Lookup(context.Background(), "player"); !errors.Is(err, ErrActorNotFound) {
		t.Fatalf("lookup after successful retry = %v", err)
	}
}

func TestStopHookReceivesDeadlineAndCanRetryWithFreshContext(t *testing.T) {
	system := NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	scope, _ := system.NewScope("stop-deadline")
	var saves atomic.Int32
	definition, err := Define(scope, func(context.Context, string) (*persistenceTestActor, error) {
		return &persistenceTestActor{save: func(ctx context.Context) error {
			if saves.Add(1) == 1 {
				<-ctx.Done()
			}
			return ctx.Err()
		}}, nil
	}, DefinitionOptions{QueueCapacity: 8})
	if err != nil {
		t.Fatal(err)
	}
	instance, _ := definition.GetOrCreate(context.Background(), "player")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	result, err := Command(context.Background(), instance.Ref(), func(execution Context) (StopResult, error) {
		return execution.StopSelf(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Wait(context.Background()); !errors.Is(err, ErrStopFailed) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline result = %v", err)
	}
	if err := definition.Stop(context.Background(), "player"); err != nil {
		t.Fatalf("retry with fresh context = %v", err)
	}
}

func TestConcurrentForceStopsFinalizeQuarantinedActors(t *testing.T) {
	for _, timing := range []string{"after-loop-exit", "racing-panic"} {
		t.Run(timing, func(t *testing.T) {
			system := NewActorSystem(time.Second)
			system.panicThreshold = 1
			t.Cleanup(func() { _ = system.Stop(context.Background()) })
			scope, _ := system.NewScope("force-quarantine")
			var saves atomic.Int32
			definition, err := Define(scope, func(context.Context, int) (*persistenceTestActor, error) {
				return &persistenceTestActor{save: func(context.Context) error {
					saves.Add(1)
					return nil
				}}, nil
			}, DefinitionOptions{QueueCapacity: 8})
			if err != nil {
				t.Fatal(err)
			}
			for id := range 16 {
				instance, err := definition.GetOrCreate(context.Background(), id)
				if err != nil {
					t.Fatal(err)
				}
				ref := instance.Ref()
				cell, err := ref.resolveCell()
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				release := make(chan struct{})
				releaseOnce := sync.OnceFunc(func() { close(release) })
				t.Cleanup(releaseOnce)
				stopping := make(chan StopResult, 1)
				panicked := make(chan error, 1)
				go func() {
					_, err := Command(ctx, ref, func(execution Context) (struct{}, error) {
						result, err := execution.StopSelf(ctx)
						if err != nil {
							return struct{}{}, err
						}
						stopping <- result
						<-release
						panic("quarantine during stop")
					})
					panicked <- err
				}()
				var original StopResult
				select {
				case original = <-stopping:
				case <-ctx.Done():
					t.Fatal("handler did not request its own stop")
				}
				if timing == "after-loop-exit" {
					releaseOnce()
					select {
					case <-cell.done:
					case <-ctx.Done():
						t.Fatal("quarantined loop did not exit")
					}
				}
				const waiters = 8
				start := make(chan struct{})
				forced := make(chan error, waiters)
				for range waiters {
					go func() { <-start; forced <- ref.ForceStop(ctx) }()
				}
				close(start)
				releaseOnce()
				for range waiters {
					if err := <-forced; err != nil {
						t.Fatalf("ForceStop = %v", err)
					}
				}
				if err := <-panicked; !errors.Is(err, ErrHandlerPanic) {
					t.Fatalf("panicking command = %v", err)
				}
				if err := original.Wait(ctx); err != nil && !errors.Is(err, ErrActorQuarantined) {
					t.Fatalf("original StopResult = %v", err)
				}
				if _, err := definition.Lookup(ctx, id); !errors.Is(err, ErrActorNotFound) {
					t.Fatalf("forced instance remained registered: %v", err)
				}
				rebuilt, err := definition.GetOrCreate(ctx, id)
				if err != nil || rebuilt == instance {
					t.Fatalf("rebuild after force = %p, %v", rebuilt, err)
				}
				if _, err := Query(ctx, ref, func(Context) (struct{}, error) { return struct{}{}, nil }); !errors.Is(err, ErrStaleActorRef) {
					t.Fatalf("old reference after rebuild = %v", err)
				}
				cancel()
			}
			if saves.Load() != 0 {
				t.Fatalf("forced actors ran %d stop hooks", saves.Load())
			}
		})
	}
}

func TestForceStopPreservesFailedPersistenceResult(t *testing.T) {
	system := NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	scope, _ := system.NewScope("force-after-save-failure")
	failure := errors.New("save unavailable")
	var saves atomic.Int32
	definition, err := Define(scope, func(context.Context, string) (*persistenceTestActor, error) {
		return &persistenceTestActor{save: func(context.Context) error {
			saves.Add(1)
			return failure
		}}, nil
	}, DefinitionOptions{QueueCapacity: 8})
	if err != nil {
		t.Fatal(err)
	}
	instance, err := definition.GetOrCreate(context.Background(), "player")
	if err != nil {
		t.Fatal(err)
	}
	result, err := Command(context.Background(), instance.Ref(), func(execution Context) (StopResult, error) {
		return execution.StopSelf(context.Background())
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Wait(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("failed persistence result = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := instance.Ref().ForceStop(ctx); err != nil {
		t.Fatalf("ForceStop after save failure = %v", err)
	}
	if err := result.Wait(ctx); !errors.Is(err, ErrStopFailed) || !errors.Is(err, failure) {
		t.Fatalf("ForceStop changed the original persistence result: %v", err)
	}
	if saves.Load() != 1 {
		t.Fatalf("ForceStop retried persistence: saves = %d", saves.Load())
	}
	if _, err := definition.Lookup(ctx, "player"); !errors.Is(err, ErrActorNotFound) {
		t.Fatalf("failed instance remained registered after ForceStop: %v", err)
	}
}

func TestCanceledCommandIsNotAcceptedAndNilInterfaceResultIsValid(t *testing.T) {
	system := NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	scope, _ := system.NewScope("command-admission")
	definition, _ := Define(scope, func(context.Context, string) (*systemTestActor, error) {
		return &systemTestActor{}, nil
	}, DefinitionOptions{QueueCapacity: 8})
	instance, _ := definition.GetOrCreate(context.Background(), "player")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Command(ctx, instance.Ref(), func(Context) (any, error) {
		t.Error("canceled command executed")
		return nil, nil
	})
	if !errors.Is(err, context.Canceled) || system.Metrics().AcceptedTasks != 0 {
		t.Fatalf("canceled admission = %v, metrics = %#v", err, system.Metrics())
	}
	value, err := Command(context.Background(), instance.Ref(), func(Context) (any, error) { return nil, nil })
	if value != nil || err != nil {
		t.Fatalf("nil interface result = %v, %v", value, err)
	}
}

func TestForceStopSelfResultWaitsForHandlerAndSurvivesReplacement(t *testing.T) {
	system := NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	scope, _ := system.NewScope("force-stop-self")
	var saves atomic.Int32
	definition, err := Define(scope, func(context.Context, string) (*persistenceTestActor, error) {
		return &persistenceTestActor{save: func(context.Context) error {
			saves.Add(1)
			return nil
		}}, nil
	}, DefinitionOptions{QueueCapacity: 8})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	instance, err := definition.GetOrCreate(ctx, "team")
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseOnce)
	stopping := make(chan StopResult, 1)
	completed := make(chan error, 1)
	go func() {
		_, err := Command(ctx, instance.Ref(), func(execution Context) (struct{}, error) {
			result, err := execution.ForceStopSelf()
			if err != nil {
				return struct{}{}, err
			}
			stopping <- result
			<-release
			return struct{}{}, nil
		})
		completed <- err
	}()
	var result StopResult
	select {
	case result = <-stopping:
	case <-ctx.Done():
		t.Fatal("handler did not request its own force stop")
	}
	waitContext, cancelWait := context.WithCancel(ctx)
	cancelWait()
	if err := result.Wait(waitContext); !errors.Is(err, context.Canceled) || !errors.Is(err, ErrActorStopping) {
		t.Fatalf("wait while handler is running = %v", err)
	}
	releaseOnce()
	if err := result.Wait(ctx); err != nil {
		t.Fatalf("wait after handler returns = %v", err)
	}
	if err := <-completed; err != nil {
		t.Fatal(err)
	}
	if saves.Load() != 0 {
		t.Fatal("self force stop ran the persistence hook")
	}
	replacement, err := definition.GetOrCreate(ctx, "team")
	if err != nil || replacement == instance {
		t.Fatalf("replacement = %p, %v", replacement, err)
	}
	if err := result.Wait(ctx); err != nil {
		t.Fatalf("original stop result after replacement = %v", err)
	}
	if _, err := Query(ctx, replacement.Ref(), func(Context) (struct{}, error) { return struct{}{}, nil }); err != nil {
		t.Fatalf("original stop result affected the replacement: %v", err)
	}
}
