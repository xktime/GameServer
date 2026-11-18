package team

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type pausedBroadcastContext struct {
	context.Context
	deadlines atomic.Int32
	paused    chan struct{}
	release   chan struct{}
}

func (c *pausedBroadcastContext) Deadline() (time.Time, bool) {
	if c.deadlines.Add(1) == 2 {
		close(c.paused)
		<-c.release
	}
	return time.Now().Add(time.Hour), true
}

func TestTeamStopDoesNotWaitForUnrelatedBroadcast(t *testing.T) {
	registry := newTeamRepositoryTestRegistry(t, &fakeTeamRepository{})
	for _, id := range []int64{7, 8} {
		if _, err := registry.PrepareLogin(context.Background(), id, 42, true); err != nil {
			t.Fatal(err)
		}
	}
	started, blocked := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(blocked) })
	t.Cleanup(release)
	if err := registry.Broadcast(context.Background(), 7, func(context.Context, int64) error {
		close(started)
		<-blocked
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := registry.Stop(ctx, 8); err != nil {
		t.Fatalf("Team 8 Stop depended on Team 7 delivery: %v", err)
	}
}

func TestTeamStopIncludesBroadcastBetweenSnapshotAndAdmission(t *testing.T) {
	registry := newTeamRepositoryTestRegistry(t, &fakeTeamRepository{})
	ctx := context.Background()
	if _, err := registry.PrepareLogin(ctx, 7, 42, true); err != nil {
		t.Fatal(err)
	}
	paused := &pausedBroadcastContext{Context: ctx, paused: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(paused.release) })
	t.Cleanup(release)
	delivered := make(chan struct{}, 1)
	submitted := make(chan error, 1)
	go func() {
		submitted <- registry.Broadcast(paused, 7, func(context.Context, int64) error {
			delivered <- struct{}{}
			return nil
		})
	}()
	select {
	case <-paused.paused:
	case <-time.After(time.Second):
		t.Fatal("broadcast did not reach admission")
	}
	stopped := make(chan error, 1)
	go func() { stopped <- registry.Stop(ctx, 7) }()
	select {
	case err := <-stopped:
		release()
		submitErr := <-submitted
		if err != nil {
			t.Fatal(err)
		}
		if submitErr == nil {
			t.Fatal("broadcast was admitted after Team Stop returned success")
		}
	case <-time.After(20 * time.Millisecond):
		release()
		if err := <-submitted; err != nil && !errors.Is(err, actor.ErrActorStopping) && !errors.Is(err, actor.ErrActorStopped) {
			t.Fatal(err)
		}
		if err := <-stopped; err != nil {
			t.Fatal(err)
		}
	}
	// Reusing the ID must get its own broadcast lifetime.
	if _, err := registry.PrepareLogin(ctx, 7, 43, true); err != nil {
		t.Fatal(err)
	}
	if err := registry.Broadcast(ctx, 7, func(_ context.Context, id int64) error {
		if id != 43 {
			t.Errorf("replacement Team broadcast used old member %d", id)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Stop(ctx, 7); err != nil {
		t.Fatal(err)
	}
}

func TestLastLeaveWaitsForItsBroadcasts(t *testing.T) {
	registry := newTeamRepositoryTestRegistry(t, &fakeTeamRepository{})
	ctx := context.Background()
	if _, err := registry.PrepareLogin(ctx, 7, 42, true); err != nil {
		t.Fatal(err)
	}
	started, blocked := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(blocked) })
	t.Cleanup(release)
	if err := registry.Broadcast(ctx, 7, func(context.Context, int64) error {
		close(started)
		<-blocked
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	stopped := make(chan error, 1)
	go func() { stopped <- registry.Leave(ctx, 7, 42) }()
	select {
	case err := <-stopped:
		t.Fatalf("last Leave returned before its broadcast completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	release()
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
}

func TestAbortRetryWaitsForBroadcastsOfOriginalGeneration(t *testing.T) {
	repository := &fakeTeamRepository{}
	registry := newTeamRepositoryTestRegistry(t, repository)
	ctx := context.Background()
	prepared, err := registry.PrepareLogin(ctx, 7, 42, true)
	if err != nil {
		t.Fatal(err)
	}
	started, blocked := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(blocked) })
	t.Cleanup(release)
	if err := registry.Broadcast(ctx, 7, func(context.Context, int64) error {
		close(started)
		<-blocked
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	waitContext, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if err := prepared.Abort(waitContext); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Abort during pending broadcast = %v", err)
	}
	if _, err := registry.PrepareLogin(ctx, 7, 43, true); err != nil {
		t.Fatal(err)
	}
	retried := make(chan error, 1)
	go func() { retried <- prepared.Abort(ctx) }()
	select {
	case err := <-retried:
		t.Fatalf("old Abort retry ignored pending broadcast: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	release()
	if err := <-retried; err != nil {
		t.Fatal(err)
	}
	if len(repository.saved) != 0 || len(repository.deletedIDs) != 0 {
		t.Fatal("login abort persisted or deleted durable Team data")
	}
	snapshot, err := registry.Snapshot(ctx, 7)
	if err != nil || len(snapshot.MemberIDs) != 1 || snapshot.MemberIDs[0] != 43 {
		t.Fatalf("old Abort changed replacement Team: %+v, %v", snapshot, err)
	}
}

func TestRecoveryDrainsTeamBroadcastsBeforeCleanup(t *testing.T) {
	registry := newTeamRepositoryTestRegistry(t, &fakeTeamRepository{})
	ctx := context.Background()
	if _, err := registry.PrepareLogin(ctx, 7, 42, true); err != nil {
		t.Fatal(err)
	}
	instance, _ := registry.lookup(ctx, 7)
	t.Cleanup(func() { _ = instance.Ref().ForceStop(ctx) })
	started, blocked := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(blocked) })
	t.Cleanup(release)
	delivered := make(chan struct{})
	if err := registry.Broadcast(ctx, 7, func(context.Context, int64) error {
		close(started)
		<-blocked
		close(delivered)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	for range 3 {
		_, err := actor.Command(ctx, instance.Ref(), func(actor.Context) (struct{}, error) { panic("quarantine") })
		if !errors.Is(err, actor.ErrHandlerPanic) {
			t.Fatal(err)
		}
	}
	cleanup := func(context.Context) error {
		select {
		case <-delivered:
		default:
			t.Error("recovery cleaned up recipients before broadcast delivery")
		}
		return nil
	}
	waitContext, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if err := registry.Recover(waitContext, 7, cleanup); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("recovery during broadcast = %v", err)
	}
	release()
	if err := registry.Recover(ctx, 7, cleanup); err != nil {
		t.Fatal(err)
	}
}
