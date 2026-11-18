package team

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"sync"
	"testing"
	"time"
)

func TestRecoveryRequiresCleanupAndReloadsTeamOnNextLogin(t *testing.T) {
	repository := &fakeTeamRepository{loaded: &Team{TeamId: 7, LeaderId: 42, TeamMembers: []int64{42}, RoomId: "persisted"}}
	registry := newTeamRepositoryTestRegistry(t, repository)
	ctx := context.Background()
	if _, err := registry.PrepareLogin(ctx, 7, 42, false); err != nil {
		t.Fatal(err)
	}
	previous, _ := registry.lookup(ctx, 7)
	t.Cleanup(func() { _ = previous.Ref().ForceStop(ctx) })
	if err := registry.SetRoomProjection(ctx, 7, "unsaved"); err != nil {
		t.Fatal(err)
	}
	if err := registry.Recover(ctx, 7, func(context.Context) error { t.Fatal("healthy team was cleaned"); return nil }); !errors.Is(err, actor.ErrActorNotQuarantined) {
		t.Fatalf("healthy recovery = %v", err)
	}
	for range 3 {
		_, err := actor.Command(ctx, previous.Ref(), func(actor.Context) (struct{}, error) { panic("quarantine") })
		if !errors.Is(err, actor.ErrHandlerPanic) {
			t.Fatal(err)
		}
	}
	cleanupFailure := errors.New("room cleanup failed")
	if err := registry.Recover(ctx, 7, func(context.Context) error { return cleanupFailure }); !errors.Is(err, cleanupFailure) {
		t.Fatal(err)
	}
	if _, err := registry.Snapshot(ctx, 7); !errors.Is(err, actor.ErrActorQuarantined) {
		t.Fatalf("failed cleanup lost quarantine: %v", err)
	}
	if err := registry.Recover(ctx, 7, func(context.Context) error {
		if _, err := registry.Snapshot(ctx, 7); !errors.Is(err, actor.ErrActorQuarantined) {
			t.Errorf("cleanup must precede disposal: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.lookup(ctx, 7); !errors.Is(err, actor.ErrActorNotFound) || len(repository.saved) != 0 {
		t.Fatalf("recovery activated or saved an unsafe replacement: lookup=%v saves=%v", err, repository.saved)
	}
	if _, err := registry.PrepareLogin(ctx, 7, 42, false); err != nil {
		t.Fatal(err)
	}
	snapshot, err := registry.Snapshot(ctx, 7)
	if err != nil || snapshot.RoomID != "persisted" || len(repository.loadIDs) != 2 {
		t.Fatalf("next login did not reload durable Team data: %+v, %v, loads=%v", snapshot, err, repository.loadIDs)
	}
}

func TestRecoveryWaitHonorsDeadline(t *testing.T) {
	registry := newTeamRepositoryTestRegistry(t, &fakeTeamRepository{})
	ctx := context.Background()
	if _, err := registry.PrepareLogin(ctx, 7, 42, true); err != nil {
		t.Fatal(err)
	}
	instance, _ := registry.lookup(ctx, 7)
	t.Cleanup(func() { _ = instance.Ref().ForceStop(ctx) })
	for range 3 {
		_, err := actor.Command(ctx, instance.Ref(), func(actor.Context) (struct{}, error) { panic("quarantine") })
		if !errors.Is(err, actor.ErrHandlerPanic) {
			t.Fatal(err)
		}
	}
	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	first := make(chan error, 1)
	go func() {
		first <- registry.Recover(ctx, 7, func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	t.Cleanup(func() { unblock(); <-first })
	<-entered
	waitContext, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	second := make(chan error, 1)
	go func() {
		second <- registry.Recover(waitContext, 7, func(context.Context) error {
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
		t.Fatal("recovery kept waiting for the lock after its deadline")
	}
}

func TestRecoveryDoesNotBlockDifferentTeam(t *testing.T) {
	registry := newTeamRepositoryTestRegistry(t, &fakeTeamRepository{})
	ctx := context.Background()
	for _, teamID := range []int64{7, 8} {
		if _, err := registry.PrepareLogin(ctx, teamID, 42, true); err != nil {
			t.Fatal(err)
		}
		instance, _ := registry.lookup(ctx, teamID)
		t.Cleanup(func() { _ = instance.Ref().ForceStop(ctx) })
		for range 3 {
			_, err := actor.Command(ctx, instance.Ref(), func(actor.Context) (struct{}, error) { panic("quarantine") })
			if !errors.Is(err, actor.ErrHandlerPanic) {
				t.Fatal(err)
			}
		}
	}
	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	first := make(chan error, 1)
	go func() {
		first <- registry.Recover(ctx, 7, func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	t.Cleanup(func() { unblock(); <-first })
	<-entered
	otherContext, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := registry.Recover(otherContext, 8, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("unrelated Team recovery blocked: %v", err)
	}
	if _, err := registry.lookup(ctx, 8); !errors.Is(err, actor.ErrActorNotFound) {
		t.Fatalf("unrelated Team was not disposed: %v", err)
	}
}
