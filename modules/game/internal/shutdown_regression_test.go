package internal

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"gameserver/modules/game/internal/managers/team"
	"sync"
	"testing"
	"time"
)

type retryShutdownWorker struct {
	contexts []context.Context
	stopped  bool
}

type shutdownTeamRepository struct{}

func (shutdownTeamRepository) FindByID(context.Context, int64) (*team.Team, error) { return nil, nil }
func (shutdownTeamRepository) Save(context.Context, *team.Team) error              { return nil }
func (shutdownTeamRepository) DeleteByID(context.Context, int64) error             { return nil }

type stoppedProvisionWorker struct{}

func (stoppedProvisionWorker) Start()                     {}
func (stoppedProvisionWorker) Stop(context.Context) error { return nil }

type broadcastRecipient struct {
	actor.BaseActor
}

type signalingBroadcastStopper struct {
	teams   *team.Registry
	started chan struct{}
}

func (s signalingBroadcastStopper) StopBroadcasts(ctx context.Context) error {
	close(s.started)
	return s.teams.StopBroadcasts(ctx)
}

func TestModuleDrainsBroadcastsWhileRecipientsRemainAvailable(t *testing.T) {
	system := actor.NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	scope, _ := system.NewScope("broadcast-shutdown")
	teams, err := team.NewRegistryWithRepository(scope, shutdownTeamRepository{})
	if err != nil {
		t.Fatal(err)
	}
	recipients, err := actor.Define(scope, func(context.Context, int64) (*broadcastRecipient, error) {
		return &broadcastRecipient{}, nil
	}, actor.DefinitionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recipients.GetOrCreate(context.Background(), 42); err != nil {
		t.Fatal(err)
	}
	if _, err := teams.PrepareLogin(context.Background(), 7, 42, true); err != nil {
		t.Fatal(err)
	}
	started, blocked := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(blocked) })
	t.Cleanup(release)
	delivered := make(chan error, 1)
	if err := teams.Broadcast(context.Background(), 7, func(ctx context.Context, id int64) error {
		close(started)
		<-blocked
		recipient, err := recipients.Lookup(ctx, id)
		if err == nil {
			_, err = actor.Command(ctx, recipient.Ref(), func(actor.Context) (struct{}, error) { return struct{}{}, nil })
		}
		delivered <- err
		return err
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	draining := signalingBroadcastStopper{teams: teams, started: make(chan struct{})}
	stopped := make(chan error, 1)
	go func() { stopped <- NewModule(scope, nil, stoppedProvisionWorker{}, draining).Shutdown() }()
	select {
	case <-draining.started:
	case <-time.After(time.Second):
		t.Fatal("module did not start draining broadcasts before stopping recipients")
	}
	select {
	case err := <-stopped:
		t.Fatalf("module stopped before dispatch completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	release()
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if err := <-delivered; err != nil {
		t.Fatalf("recipient unavailable during admitted broadcast: %v", err)
	}
	if err := teams.Broadcast(context.Background(), 7, func(context.Context, int64) error {
		t.Error("broadcast admitted after module shutdown")
		return nil
	}); !errors.Is(err, actor.ErrScopeStopped) {
		t.Fatalf("broadcast after shutdown = %v", err)
	}
}

type noopBroadcastStopper struct{}

func (noopBroadcastStopper) StopBroadcasts(context.Context) error { return nil }

func (*retryShutdownWorker) Start() {}

func (w *retryShutdownWorker) Stop(ctx context.Context) error {
	w.contexts = append(w.contexts, ctx)
	if len(w.contexts) == 1 {
		return context.DeadlineExceeded
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	w.stopped = true
	return nil
}

type provisionShutdownActor struct {
	actor.BaseActor
	worker             *retryShutdownWorker
	stoppedAfterWorker bool
}

func (a *provisionShutdownActor) OnStop(context.Context) error {
	a.stoppedAfterWorker = a.worker.stopped
	return nil
}

func TestModuleWaitsForProvisionWorkerBeforeStoppingScope(t *testing.T) {
	system := actor.NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	scope, _ := system.NewScope("provision-shutdown")
	worker := &retryShutdownWorker{}
	definition, err := actor.Define(scope, func(context.Context, string) (*provisionShutdownActor, error) {
		return &provisionShutdownActor{worker: worker}, nil
	}, actor.DefinitionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	instance, err := definition.GetOrCreate(context.Background(), "user")
	if err != nil {
		t.Fatal(err)
	}
	NewModule(scope, nil, worker, noopBroadcastStopper{}).OnDestroy()
	if len(worker.contexts) != 2 || !instance.stoppedAfterWorker {
		t.Fatalf("Stop calls = %d; scope stopped after worker = %t", len(worker.contexts), instance.stoppedAfterWorker)
	}
	initialDeadline, _ := worker.contexts[0].Deadline()
	finalDeadline, ok := worker.contexts[1].Deadline()
	if !ok || !finalDeadline.After(initialDeadline) {
		t.Fatal("worker retry did not receive a fresh shutdown budget")
	}
}
