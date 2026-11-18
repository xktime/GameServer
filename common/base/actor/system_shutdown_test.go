package actor_test

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type shutdownTestActor struct {
	actor.BaseActor
	save func(context.Context) error
}

func (a *shutdownTestActor) OnStop(ctx context.Context) error { return a.save(ctx) }

type shutdownWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *shutdownWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func TestSystemStopRetriesExpiredPendingSave(t *testing.T) {
	storageFailure := errors.New("storage unavailable")
	for _, test := range []struct {
		name       string
		initialErr error
		retryErr   error
	}{
		{name: "canceled retry succeeds", initialErr: context.Canceled},
		{name: "canceled retry fails", initialErr: context.Canceled, retryErr: storageFailure},
		{name: "deadline retry succeeds", initialErr: context.DeadlineExceeded},
		{name: "deadline retry fails", initialErr: context.DeadlineExceeded, retryErr: storageFailure},
	} {
		t.Run(test.name, func(t *testing.T) {
			system := actor.NewActorSystem(time.Second)
			scope, _ := system.NewScope("final-shutdown")
			var saves atomic.Int32
			definition, err := actor.Define(scope, func(context.Context, int) (*shutdownTestActor, error) {
				return &shutdownTestActor{save: func(ctx context.Context) error {
					if saves.Add(1) == 1 {
						return ctx.Err()
					}
					return test.retryErr
				}}, nil
			}, actor.DefinitionOptions{})
			if err != nil {
				t.Fatal(err)
			}
			instance, err := definition.GetOrCreate(context.Background(), 42)
			if err != nil {
				t.Fatal(err)
			}
			release := make(chan struct{})
			releaseOnce := sync.OnceFunc(func() { close(release) })
			t.Cleanup(func() {
				releaseOnce()
				_ = instance.Ref().ForceStop(context.Background())
				_ = system.Stop(context.Background())
			})
			moduleContext, cancelModule := context.WithCancel(context.Background())
			if test.initialErr == context.DeadlineExceeded {
				cancelModule()
				moduleContext, cancelModule = context.WithTimeout(context.Background(), 20*time.Millisecond)
			}
			defer cancelModule()
			stopping := make(chan actor.StopResult, 1)
			commandDone := make(chan error, 1)
			go func() {
				_, err := actor.Command(context.Background(), instance.Ref(), func(execution actor.Context) (struct{}, error) {
					result, err := execution.StopSelf(moduleContext)
					stopping <- result
					<-release
					return struct{}{}, err
				})
				commandDone <- err
			}()
			original := <-stopping
			if test.initialErr == context.Canceled {
				cancelModule()
			}
			<-moduleContext.Done()
			if err := scope.Stop(moduleContext); !errors.Is(err, test.initialErr) {
				t.Fatalf("module Stop = %v", err)
			}
			finalContext, cancelFinal := context.WithTimeout(context.Background(), time.Second)
			defer cancelFinal()
			waitingContext := &shutdownWaitContext{Context: finalContext, waiting: make(chan struct{})}
			finalDone := make(chan error, 1)
			go func() { finalDone <- system.Stop(waitingContext) }()
			select {
			case <-waitingContext.waiting:
			case <-finalContext.Done():
				t.Fatal("final shutdown did not join the pending stop")
			}
			releaseOnce()
			err = <-finalDone
			if !errors.Is(err, test.retryErr) || errors.Is(err, test.initialErr) {
				t.Fatalf("final shutdown = %v; want %v", err, test.retryErr)
			}
			if saves.Load() != 2 {
				t.Fatalf("save attempts = %d; want 2", saves.Load())
			}
			if err := <-commandDone; err != nil {
				t.Fatal(err)
			}
			if err := original.Wait(finalContext); !errors.Is(err, actor.ErrStopFailed) || !errors.Is(err, test.initialErr) {
				t.Fatalf("original stop result changed: %v", err)
			}
			if test.retryErr == nil {
				if _, err := actor.Query(context.Background(), instance.Ref(), func(actor.Context) (bool, error) {
					return true, nil
				}); !errors.Is(err, actor.ErrActorStopped) {
					t.Fatalf("saved instance remained registered: %v", err)
				}
			}
		})
	}
}

func TestSystemStopDoesNotRetryAfterBudgetExpires(t *testing.T) {
	system := actor.NewActorSystem(time.Second)
	scope, _ := system.NewScope("final-shutdown-deadline")
	var saves atomic.Int32
	started := make(chan struct{})
	definition, err := actor.Define(scope, func(context.Context, int) (*shutdownTestActor, error) {
		return &shutdownTestActor{save: func(ctx context.Context) error {
			saves.Add(1)
			close(started)
			<-ctx.Done()
			return ctx.Err()
		}}, nil
	}, actor.DefinitionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	instance, err := definition.GetOrCreate(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = instance.Ref().ForceStop(context.Background())
		_ = system.Stop(context.Background())
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- system.Stop(ctx) }()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("final shutdown = %v", err)
	}
	if saves.Load() != 1 {
		t.Fatalf("save attempts after budget expired = %d; want 1", saves.Load())
	}
}

func TestSystemStopDoesNotRetrySaveFailureWithinBudget(t *testing.T) {
	for _, failure := range []error{errors.New("storage unavailable"), context.Canceled, context.DeadlineExceeded} {
		t.Run(failure.Error(), func(t *testing.T) {
			system := actor.NewActorSystem(time.Second)
			scope, _ := system.NewScope("final-shutdown-failure")
			var saves atomic.Int32
			definition, err := actor.Define(scope, func(context.Context, int) (*shutdownTestActor, error) {
				return &shutdownTestActor{save: func(context.Context) error {
					saves.Add(1)
					return failure
				}}, nil
			}, actor.DefinitionOptions{})
			if err != nil {
				t.Fatal(err)
			}
			instance, err := definition.GetOrCreate(context.Background(), 42)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = instance.Ref().ForceStop(context.Background())
				_ = system.Stop(context.Background())
			})
			if err := system.Stop(context.Background()); !errors.Is(err, actor.ErrStopFailed) || !errors.Is(err, failure) {
				t.Fatalf("final shutdown = %v; want %v", err, failure)
			}
			if saves.Load() != 1 {
				t.Fatalf("save attempts within budget = %d; want 1", saves.Load())
			}
		})
	}
}
