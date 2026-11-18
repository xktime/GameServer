package internal

import (
	"context"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"testing"
	"time"
)

type retryShutdownLogin struct {
	contexts []context.Context
	closed   bool
}

func (*retryShutdownLogin) SubmitLogin(*message.C2S_Login, gate.ConnectionAgent, uint32) error {
	return nil
}

func (l *retryShutdownLogin) Close(ctx context.Context) error {
	l.contexts = append(l.contexts, ctx)
	if len(l.contexts) == 1 {
		return context.DeadlineExceeded
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	l.closed = true
	return nil
}

type shutdownLoginJob struct{}

func (shutdownLoginJob) Stop(context.Context) error { return nil }

type shutdownConnectionActor struct {
	actor.BaseActor
	login             *retryShutdownLogin
	stoppedAfterLogin bool
}

func (a *shutdownConnectionActor) OnStop(context.Context) error {
	a.stoppedAfterLogin = a.login.closed
	return nil
}

func TestModuleWaitsAgainForAuthenticationBeforeStoppingScope(t *testing.T) {
	system := actor.NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	scope, err := system.NewScope("login")
	if err != nil {
		t.Fatal(err)
	}
	login := &retryShutdownLogin{}
	connections, err := actor.Define(scope, func(context.Context, string) (*shutdownConnectionActor, error) {
		return &shutdownConnectionActor{login: login}, nil
	}, actor.DefinitionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := connections.GetOrCreate(context.Background(), "connection")
	if err != nil {
		t.Fatal(err)
	}
	m := &Module{scope: scope, login: login, job: shutdownLoginJob{}}
	m.OnDestroy()
	if len(login.contexts) != 2 || !connection.stoppedAfterLogin {
		t.Fatalf("Close calls = %d; scope stopped after authentication = %t", len(login.contexts), connection.stoppedAfterLogin)
	}
	initialDeadline, _ := login.contexts[0].Deadline()
	finalDeadline, ok := login.contexts[1].Deadline()
	if !ok || !finalDeadline.After(initialDeadline) {
		t.Fatalf("final authentication wait did not receive a fresh budget: %v, %v", initialDeadline, finalDeadline)
	}
}
