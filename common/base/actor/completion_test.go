package actor

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCallUntilDoneKeepsAcceptedTaskOwnershipAfterCancellation(t *testing.T) {
	system := NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	scope, _ := system.NewScope("completion")
	definition, err := Define(scope, Test1, func(context.Context, string) (*systemTestActor, error) { return &systemTestActor{}, nil })
	if err != nil {
		t.Fatal(err)
	}
	instance, err := definition.GetOrCreate(context.Background(), "player")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		value, err := CallUntilDone(ctx, instance.Ref(), func(execution Context) (int, error) {
			close(started)
			<-release
			if execution.Err() != nil {
				return 0, execution.Err()
			}
			return 42, nil
		})
		if err == nil && value != 42 {
			err = errors.New("missing definitive result")
		}
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		t.Fatalf("returned before accepted task completed: %v", err)
	default:
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCallUntilDoneReportsPanicsAndPreservesCycleDetection(t *testing.T) {
	system := NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	scope, _ := system.NewScope("completion")
	definition, err := Define(scope, Test1, func(context.Context, string) (*systemTestActor, error) { return &systemTestActor{}, nil })
	if err != nil {
		t.Fatal(err)
	}
	first, _ := definition.GetOrCreate(context.Background(), "first")
	second, _ := definition.GetOrCreate(context.Background(), "second")
	_, err = CallUntilDone(context.Background(), first.Ref(), func(Context) (int, error) { panic("failed prepare") })
	if !errors.Is(err, ErrHandlerPanic) {
		t.Fatalf("panic outcome = %v", err)
	}
	_, err = CallUntilDone(context.Background(), first.Ref(), func(ctx Context) (int, error) {
		return CallUntilDone(ctx, second.Ref(), func(ctx Context) (int, error) {
			return CallUntilDone(ctx, first.Ref(), func(Context) (int, error) { return 0, nil })
		})
	})
	if !errors.Is(err, ErrCallCycle) {
		t.Fatalf("cycle outcome = %v", err)
	}
}
