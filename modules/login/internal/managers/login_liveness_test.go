package managers

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/modules/game/playersession"
	"gameserver/modules/login/internal/models"
	"sync"
	"testing"
	"time"
)

type immediateAuthenticator struct{}

func (immediateAuthenticator) Authenticate(context.Context, gate.Agent, *message.C2S_Login) *models.LoginResponse {
	return &models.LoginResponse{Openid: "open-id"}
}

type slowUserLogin struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *slowUserLogin) UserLogin(attempt *playersession.Attempt, openID string, serverID int32, loginType message.LoginType) error {
	s.once.Do(func() { close(s.started); <-s.release })
	return (fakeUserLoginService{}).UserLogin(attempt, openID, serverID, loginType)
}

func TestSlowUserPreparationDoesNotBlockConnectionLifecycle(t *testing.T) {
	system := actor.NewActorSystem(time.Second)
	scope, _ := system.NewScope("slow-login")
	connections, err := NewConnectManager(context.Background(), scope, playersession.NewState())
	if err != nil {
		t.Fatal(err)
	}
	users := &slowUserLogin{started: make(chan struct{}), release: make(chan struct{})}
	manager, err := NewLoginManager(context.Background(), scope, users, LoginManagerOptions{Authenticator: immediateAuthenticator{}, Workers: 1, QueueSize: 2, Attempts: connections})
	if err != nil {
		t.Fatal(err)
	}
	release := sync.OnceFunc(func() { close(users.release) })
	defer func() { release(); manager.Close(); _ = system.Stop(context.Background()) }()
	first := &asyncLoginAgent{testConnectionAgent: testConnectionAgent{id: 1}, writes: make(chan loginWrite, 2)}
	second := &asyncLoginAgent{testConnectionAgent: testConnectionAgent{id: 2}, writes: make(chan loginWrite, 2)}
	connections.Register(first)
	connections.Register(second)
	if err := manager.SubmitLogin(&message.C2S_Login{ServerId: 7}, first, 1); err != nil {
		t.Fatal(err)
	}
	select {
	case <-users.started:
	case <-time.After(time.Second):
		t.Fatal("user preparation did not start")
	}
	lifecycle := make(chan error, 1)
	go func() {
		connections.UpdateHeartbeat(second.id)
		connections.RemoveClient(first.id)
		if err := manager.SubmitLogin(&message.C2S_Login{ServerId: 7}, second, 2); err != nil {
			lifecycle <- err
			return
		}
		if connections.GetActiveClients() != 1 {
			lifecycle <- errors.New("disconnect did not drain")
			return
		}
		lifecycle <- nil
	}()
	select {
	case err := <-lifecycle:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("slow User preparation blocked connection Actor")
	}
	release()
	manager.Close()
	select {
	case write := <-first.writes:
		t.Fatalf("disconnected attempt replied: %#v", write)
	default:
	}
	select {
	case write := <-second.writes:
		response, ok := write.message.(*message.S2C_Login)
		if !ok || response.LoginResult != 1 || write.sequence != 2 {
			t.Fatalf("wrong reply: %#v", write)
		}
	default:
		t.Fatal("second queued login did not complete")
	}
}

func TestQueueRejectionRepliesOnceAndInvalidatesOnlyRejectedConnection(t *testing.T) {
	system := actor.NewActorSystem(time.Second)
	scope, _ := system.NewScope("queue-rejection")
	connections, err := NewConnectManager(context.Background(), scope, playersession.NewState())
	if err != nil {
		t.Fatal(err)
	}
	auth := &blockingAuthenticator{started: make(chan struct{}), release: make(chan struct{})}
	manager, err := NewLoginManager(context.Background(), scope, fakeUserLoginService{}, LoginManagerOptions{Authenticator: auth, Workers: 1, QueueSize: 1, Timeout: time.Second, Attempts: connections})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { close(auth.release); manager.Close(); _ = system.Stop(context.Background()) }()
	agents := make([]*asyncLoginAgent, 3)
	for i := range agents {
		agents[i] = &asyncLoginAgent{testConnectionAgent: testConnectionAgent{id: gate.ConnectionID(i + 1)}, writes: make(chan loginWrite, 3)}
		connections.Register(agents[i])
	}
	if err := manager.SubmitLogin(&message.C2S_Login{ServerId: 7}, agents[0], 1); err != nil {
		t.Fatal(err)
	}
	select {
	case <-auth.started:
	case <-time.After(time.Second):
		t.Fatal("auth not started")
	}
	if err := manager.SubmitLogin(&message.C2S_Login{ServerId: 7}, agents[1], 2); err != nil {
		t.Fatal(err)
	}
	if err := manager.SubmitLogin(&message.C2S_Login{ServerId: 7}, agents[2], 3); !errors.Is(err, ErrAuthQueueFull) {
		t.Fatalf("queue rejection = %v", err)
	}
	write := <-agents[2].writes
	if response, ok := write.message.(*message.S2C_Login); !ok || response.LoginResult != -1 || write.sequence != 3 {
		t.Fatalf("rejection reply = %#v", write)
	}
	if !agents[2].closed.Load() || agents[0].closed.Load() || agents[1].closed.Load() {
		t.Fatal("rejection closed another connection")
	}
	if _, err := connections.BeginLogin(agents[2].id, 4); !errors.Is(err, playersession.ErrDisconnected) {
		t.Fatalf("rejected connection accepted login: %v", err)
	}
	select {
	case <-agents[2].writes:
		t.Fatal("duplicate failure reply")
	default:
	}
}

func TestAuthenticatedLoginRejectionRepliesWithoutClosingSession(t *testing.T) {
	system := actor.NewActorSystem(time.Second)
	scope, _ := system.NewScope("authenticated-login-rejection")
	state := playersession.NewState()
	connections, err := NewConnectManager(context.Background(), scope, state)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewLoginManager(context.Background(), scope, fakeUserLoginService{}, LoginManagerOptions{
		Authenticator: immediateAuthenticator{}, Workers: 1, QueueSize: 1, Attempts: connections,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { manager.Close(); _ = system.Stop(context.Background()) })
	agent := &asyncLoginAgent{testConnectionAgent: testConnectionAgent{id: 1}, writes: make(chan loginWrite, 3)}
	connections.Register(agent)
	if err := manager.SubmitLogin(&message.C2S_Login{ServerId: 7}, agent, 11); err != nil {
		t.Fatal(err)
	}
	select {
	case write := <-agent.writes:
		if response, ok := write.message.(*message.S2C_Login); !ok || response.LoginResult != 1 || write.sequence != 11 {
			t.Fatalf("initial login reply = %#v", write)
		}
	case <-time.After(time.Second):
		t.Fatal("initial login did not complete")
	}
	original, _ := state.Current("test")
	if err := manager.SubmitLogin(&message.C2S_Login{ServerId: 7}, agent, 12); !errors.Is(err, playersession.ErrAuthenticated) {
		t.Fatalf("repeated login = %v", err)
	}
	manager.Close()
	select {
	case write := <-agent.writes:
		if response, ok := write.message.(*message.S2C_Login); !ok || response.LoginResult != -1 || write.sequence != 12 {
			t.Fatalf("rejection reply = %#v", write)
		}
	default:
		t.Fatal("authenticated login rejection did not reply")
	}
	current, ok := state.Current("test")
	if !ok || current.Snapshot != original.Snapshot || agent.UserData() != original.Snapshot || agent.closed.Load() {
		t.Fatal("rejected login changed the current session")
	}
	select {
	case <-agent.writes:
		t.Fatal("duplicate rejection reply")
	default:
	}
}
