package actor

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type executionValueKey struct{}

var executionContextDerivations = []struct {
	name   string
	derive func(context.Context) (context.Context, context.CancelFunc)
}{
	{
		name: "WithValue",
		derive: func(ctx context.Context) (context.Context, context.CancelFunc) {
			return context.WithValue(ctx, executionValueKey{}, "derived"), func() {}
		},
	},
	{name: "WithCancel", derive: context.WithCancel},
	{
		name: "WithTimeout",
		derive: func(ctx context.Context) (context.Context, context.CancelFunc) {
			return context.WithTimeout(ctx, time.Second)
		},
	},
	{
		name: "WithoutCancel",
		derive: func(ctx context.Context) (context.Context, context.CancelFunc) {
			return context.WithoutCancel(ctx), func() {}
		},
	},
}

func newExecutionTestActor(t *testing.T, system *ActorSystem, scopeName string, ids ...string) []*systemTestActor {
	t.Helper()
	scope, err := system.NewScope(scopeName)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := Define(scope, Test1, func(context.Context, string) (*systemTestActor, error) {
		return &systemTestActor{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	instances := make([]*systemTestActor, 0, len(ids))
	for _, id := range ids {
		instance, err := definition.GetOrCreate(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		instances = append(instances, instance)
	}
	return instances
}

func TestCallRejectsCurrentActorThroughDerivedContext(t *testing.T) {
	for _, derivation := range executionContextDerivations {
		t.Run(derivation.name, func(t *testing.T) {
			system := NewActorSystem(50 * time.Millisecond)
			t.Cleanup(func() { _ = system.Stop(context.Background()) })
			instance := newExecutionTestActor(t, system, "self", "actor")[0]
			var executed atomic.Bool
			_, err := CallUntilDone(context.Background(), instance.Ref(), func(execution Context) (int, error) {
				ctx, cancel := derivation.derive(execution)
				defer cancel()
				return Call(ctx, instance.Ref(), func(Context) (int, error) {
					executed.Store(true)
					return 42, nil
				})
			})
			if !errors.Is(err, ErrCallCycle) {
				t.Fatalf("derived self call = %v; want ErrCallCycle", err)
			}
			if executed.Load() {
				t.Fatal("rejected self call executed its task")
			}
		})
	}
}

func TestDerivedContextPreservesCallChainAndValues(t *testing.T) {
	for _, derivation := range executionContextDerivations {
		t.Run(derivation.name, func(t *testing.T) {
			system := NewActorSystem(time.Second)
			t.Cleanup(func() { _ = system.Stop(context.Background()) })
			instances := newExecutionTestActor(t, system, "chain", "first", "second")
			ctx := context.WithValue(context.Background(), executionValueKey{}, "original")
			_, err := CallUntilDone(ctx, instances[0].Ref(), func(execution Context) (int, error) {
				derived, cancel := derivation.derive(execution)
				defer cancel()
				return CallUntilDone(derived, instances[1].Ref(), func(secondExecution Context) (int, error) {
					if got, want := secondExecution.Value(executionValueKey{}), derived.Value(executionValueKey{}); got != want {
						return 0, errors.New("derived context value was lost")
					}
					if secondExecution.Err() != nil {
						return 0, secondExecution.Err()
					}
					callContext, cancel := context.WithTimeout(secondExecution, 50*time.Millisecond)
					defer cancel()
					return Call(callContext, instances[0].Ref(), func(Context) (int, error) {
						return 42, nil
					})
				})
			})
			if !errors.Is(err, ErrCallCycle) {
				t.Fatalf("derived call cycle = %v; want ErrCallCycle", err)
			}
		})
	}
}

func TestCallUsesDistinctMailboxesAcrossScopesAndSystems(t *testing.T) {
	for _, separateSystems := range []bool{false, true} {
		name := "Scopes"
		if separateSystems {
			name = "Systems"
		}
		t.Run(name, func(t *testing.T) {
			firstSystem := NewActorSystem(50 * time.Millisecond)
			t.Cleanup(func() { _ = firstSystem.Stop(context.Background()) })
			secondSystem := firstSystem
			secondScope := "second"
			if separateSystems {
				secondSystem = NewActorSystem(50 * time.Millisecond)
				t.Cleanup(func() { _ = secondSystem.Stop(context.Background()) })
				secondScope = "first"
			}
			first := newExecutionTestActor(t, firstSystem, "first", "same-id")[0]
			second := newExecutionTestActor(t, secondSystem, secondScope, "same-id")[0]
			started := make(chan struct{})
			release := make(chan struct{})
			t.Cleanup(func() { close(release) })
			if err := Tell(context.Background(), second.Ref(), func(Context) error {
				close(started)
				<-release
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			<-started
			var executed atomic.Bool
			_, err := CallUntilDone(context.Background(), first.Ref(), func(execution Context) (int, error) {
				return Call(execution, second.Ref(), func(Context) (int, error) {
					executed.Store(true)
					return 42, nil
				})
			})
			if !errors.Is(err, ErrOutcomeUnknown) {
				t.Fatalf("call to blocked target = %v; want ErrOutcomeUnknown", err)
			}
			if executed.Load() {
				t.Fatal("call bypassed the target mailbox")
			}
		})
	}
}

func TestActorCanStopItselfThroughDerivedContext(t *testing.T) {
	for _, force := range []bool{false, true} {
		name := "Stop"
		if force {
			name = "ForceStop"
		}
		t.Run(name, func(t *testing.T) {
			system := NewActorSystem(time.Second)
			t.Cleanup(func() { _ = system.Stop(context.Background()) })
			instance := newExecutionTestActor(t, system, "stop", "actor")[0]
			_, err := CallUntilDone(context.Background(), instance.Ref(), func(execution Context) (int, error) {
				ctx, cancel := context.WithTimeout(execution, 50*time.Millisecond)
				defer cancel()
				if force {
					return 0, instance.Ref().ForceStop(ctx)
				}
				return 0, instance.Ref().Stop(ctx)
			})
			if err != nil {
				t.Fatalf("derived self stop = %v", err)
			}
			if err := TryTell(instance.Ref(), func(Context) error { return nil }); !errors.Is(err, ErrActorStopped) {
				t.Fatalf("Tell after self stop = %v; want ErrActorStopped", err)
			}
		})
	}
}
