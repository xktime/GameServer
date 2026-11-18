package team

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestBroadcastReturnsAfterAdmissionAndDrainsOnStopDespiteCallerCancellation(t *testing.T) {
	repository := &fakeTeamRepository{}
	registry := newTeamRepositoryTestRegistry(t, repository)
	if _, err := registry.PrepareLogin(context.Background(), 7, 42, true); err != nil {
		t.Fatal(err)
	}
	for _, playerID := range []int64{43, 44} {
		if err := registry.Join(context.Background(), 7, playerID); err != nil {
			t.Fatal(err)
		}
	}
	started, release := make(chan struct{}), make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseOnce)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var recipients []int64
	var executionErrors []error
	accepted := make(chan error, 1)
	go func() {
		accepted <- registry.Broadcast(ctx, 7, func(execution context.Context, playerID int64) error {
			recipients = append(recipients, playerID)
			executionErrors = append(executionErrors, execution.Err())
			switch playerID {
			case 42:
				close(started)
				<-release
				return actor.ErrActorNotFound
			case 43:
				return errors.New("send failed")
			default:
				return nil
			}
		})
	}()
	select {
	case err := <-accepted:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("broadcast waited for delivery")
	}
	<-started
	cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- registry.Stop(context.Background(), 7) }()
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned before delivery completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	releaseOnce()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Stop did not drain the accepted broadcast")
	}
	if !slices.Equal(recipients, []int64{42, 43, 44}) {
		t.Fatalf("broadcast recipients = %v", recipients)
	}
	for _, err := range executionErrors {
		if err != nil {
			t.Fatalf("accepted broadcast inherited caller cancellation: %v", err)
		}
	}
	if len(repository.saved) != 1 {
		t.Fatalf("Stop saves = %d; want 1", len(repository.saved))
	}
}

func TestBroadcastDoesNotBlockTeamChangesAndUsesSubmittedMembers(t *testing.T) {
	registry := newTeamRepositoryTestRegistry(t, &fakeTeamRepository{})
	ctx := context.Background()
	if _, err := registry.PrepareLogin(ctx, 7, 42, true); err != nil {
		t.Fatal(err)
	}
	if err := registry.Join(ctx, 7, 43); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	var recipients []int64
	if err := registry.Broadcast(ctx, 7, func(_ context.Context, memberID int64) error {
		if memberID == 42 {
			close(started)
			<-release
		}
		recipients = append(recipients, memberID)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	changed := make(chan error, 1)
	go func() {
		if err := registry.Join(ctx, 7, 44); err != nil {
			changed <- err
			return
		}
		if err := registry.Leave(ctx, 7, 42); err != nil {
			changed <- err
			return
		}
		if err := registry.SetRoomProjection(ctx, 7, "room-1"); err != nil {
			changed <- err
			return
		}
		snapshot, err := registry.Snapshot(ctx, 7)
		if err == nil && (snapshot.RoomID != "room-1" || !slices.Equal(snapshot.MemberIDs, []int64{43, 44})) {
			t.Errorf("team changes = %+v", snapshot)
		}
		changed <- err
	}()
	select {
	case err := <-changed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		unblock()
		<-changed
		t.Fatal("blocked broadcast prevented Team membership/projection changes")
	}
	unblock()
	if err := registry.Stop(ctx, 7); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(recipients, []int64{42, 43}) {
		t.Fatalf("broadcast did not retain submitted members: %v", recipients)
	}
}

func TestBroadcastAdmissionIsBoundedAndPreservesOrder(t *testing.T) {
	registry := newTeamRepositoryTestRegistry(t, &fakeTeamRepository{})
	ctx := context.Background()
	if _, err := registry.PrepareLogin(ctx, 7, 42, true); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	var delivered []int
	if err := registry.Broadcast(ctx, 7, func(context.Context, int64) error {
		close(started)
		<-release
		delivered = append(delivered, 0)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	for index := range broadcastQueueCapacity {
		if err := registry.Broadcast(ctx, 7, func(context.Context, int64) error {
			delivered = append(delivered, index+1)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	waitContext, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if err := registry.Broadcast(waitContext, 7, func(context.Context, int64) error {
		t.Error("broadcast rejected at admission was delivered")
		return nil
	}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("full broadcast queue admission = %v", err)
	}
	unblock()
	if err := registry.Stop(ctx, 7); err != nil {
		t.Fatal(err)
	}
	if len(delivered) != broadcastQueueCapacity+1 {
		t.Fatalf("delivered broadcasts = %d", len(delivered))
	}
	for index, value := range delivered {
		if value != index {
			t.Fatalf("delivery %d = %d; broadcasts reordered", index, value)
		}
	}
}

func TestScopeStopDrainsBroadcasts(t *testing.T) {
	system := actor.NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	scope, err := system.NewScope("team-broadcast-stop-test")
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewRegistryWithRepository(scope, &fakeTeamRepository{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := registry.PrepareLogin(ctx, 7, 42, true); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	delivered := make(chan error, 1)
	if err := registry.Broadcast(ctx, 7, func(execution context.Context, _ int64) error {
		close(started)
		<-release
		delivered <- execution.Err()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- scope.Stop(context.Background()) }()
	select {
	case err := <-stopped:
		t.Fatalf("Scope.Stop returned before delivery: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	unblock()
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if err := <-delivered; err != nil {
		t.Fatalf("accepted delivery was canceled: %v", err)
	}
	if err := registry.Stop(context.Background(), 7); err != nil {
		t.Fatalf("Stop after Scope.Stop = %v", err)
	}
}

func TestBroadcastSenderPanicDoesNotQuarantineDispatcher(t *testing.T) {
	registry := newTeamRepositoryTestRegistry(t, &fakeTeamRepository{})
	ctx := context.Background()
	if _, err := registry.PrepareLogin(ctx, 7, 42, true); err != nil {
		t.Fatal(err)
	}
	if err := registry.Join(ctx, 7, 43); err != nil {
		t.Fatal(err)
	}
	delivered := 0
	for range 3 {
		if err := registry.Broadcast(ctx, 7, func(_ context.Context, memberID int64) error {
			if memberID == 42 {
				panic("send failed")
			}
			delivered++
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := registry.Stop(ctx, 7); err != nil {
		t.Fatal(err)
	}
	if delivered != 3 {
		t.Fatalf("successful deliveries after sender panics = %d; want 3", delivered)
	}
}
