package managers

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	"gameserver/modules/game/internal/provision"
	"sync"
	"testing"
	"time"
)

func TestUserRecoveryPreservesDependenciesAndRequiresSuccessfulCleanup(t *testing.T) {
	manager := newUserManagerRepositoryTestHarness(t, &fakeUserRepository{}, provision.NewMemoryRepository())
	previous := manager.currentActor()
	sessions := manager.LoginState()
	notified := false
	manager.SetOfflineNotifier(func(context.Context, int64) bool { notified = true; return true })
	cleanupCalls := 0
	cleanup := func(context.Context) error { cleanupCalls++; return nil }
	if err := manager.Recover(context.Background(), cleanup); !errors.Is(err, actor.ErrActorNotQuarantined) || cleanupCalls != 0 {
		t.Fatalf("healthy recovery = %v; cleanup calls = %d", err, cleanupCalls)
	}
	for range 3 {
		_, err := actor.Command(context.Background(), previous.Ref(), func(actor.Context) (struct{}, error) { panic("quarantine") })
		if !errors.Is(err, actor.ErrHandlerPanic) {
			t.Fatal(err)
		}
	}
	cleanupFailure := errors.New("session cleanup failed")
	if err := manager.Recover(context.Background(), func(context.Context) error { return cleanupFailure }); !errors.Is(err, cleanupFailure) {
		t.Fatal(err)
	}
	if _, err := manager.definition.QuarantineTarget(context.Background(), "singleton"); err != nil {
		t.Fatalf("failed cleanup removed the isolated instance: %v", err)
	}
	reading, readDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			select {
			case <-reading:
				return
			default:
				manager.LoginState()
			}
		}
	}()
	err := manager.Recover(context.Background(), func(context.Context) error {
		if manager.LoginState() != sessions {
			t.Error("cleanup lost the shared login state")
		}
		return nil
	})
	close(reading)
	<-readDone
	if err != nil {
		t.Fatal(err)
	}
	if manager.currentActor() == previous || manager.LoginState() != sessions {
		t.Fatal("recovery failed to replace the actor while retaining shared session state")
	}
	if manager.CheckName("recovered") != message.Result_Success {
		t.Fatal("recovered manager did not handle a domain request")
	}
	manager.currentActor().offline(context.Background(), 42)
	if !notified {
		t.Fatal("recovery lost the Room offline notifier")
	}
}

func TestUserRecoveryWaitHonorsDeadline(t *testing.T) {
	manager := newUserManagerRepositoryTestHarness(t, &fakeUserRepository{}, provision.NewMemoryRepository())
	previous := manager.currentActor()
	t.Cleanup(func() { _ = previous.Ref().ForceStop(context.Background()) })
	for range 3 {
		_, err := actor.Command(context.Background(), previous.Ref(), func(actor.Context) (struct{}, error) { panic("quarantine") })
		if !errors.Is(err, actor.ErrHandlerPanic) {
			t.Fatal(err)
		}
	}
	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	first := make(chan error, 1)
	go func() {
		first <- manager.Recover(context.Background(), func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	t.Cleanup(func() { unblock(); <-first })
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	second := make(chan error, 1)
	go func() {
		second <- manager.Recover(ctx, func(context.Context) error {
			t.Error("expired waiter ran cleanup")
			return nil
		})
	}()
	select {
	case err := <-second:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("waiting recovery = %v", err)
		}
	case <-time.After(time.Second):
		unblock()
		<-second
		t.Fatal("singleton recovery kept waiting after its deadline")
	}
}
