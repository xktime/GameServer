package player

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	playermodel "gameserver/modules/game/internal/models/player"
	"gameserver/modules/game/playersession"
	"net"
	"sync"
	"testing"
	"time"
)

type loginPlayerAgent struct {
	id           gate.ConnectionID
	mu           sync.Mutex
	data         any
	writes       chan any
	loginStarted chan struct{}
	releaseLogin chan struct{}
}

func (a *loginPlayerAgent) ConnectionID() gate.ConnectionID { return a.id }
func (a *loginPlayerAgent) WriteMsg(msg any)                { a.writes <- msg }
func (a *loginPlayerAgent) WriteMsgWithSeq(msg any, seq uint32) {
	if _, ok := msg.(*message.S2C_Login); ok && a.loginStarted != nil {
		close(a.loginStarted)
		<-a.releaseLogin
	}
	a.writes <- struct {
		msg any
		seq uint32
	}{msg, seq}
}
func (*loginPlayerAgent) LocalAddr() net.Addr    { return nil }
func (*loginPlayerAgent) RemoteAddr() net.Addr   { return nil }
func (*loginPlayerAgent) Close()                 {}
func (*loginPlayerAgent) Destroy()               {}
func (a *loginPlayerAgent) UserData() any        { a.mu.Lock(); defer a.mu.Unlock(); return a.data }
func (a *loginPlayerAgent) SetUserData(data any) { a.mu.Lock(); defer a.mu.Unlock(); a.data = data }

func TestCommitOrdersSuccessBeforePushWithoutHoldingSessionLock(t *testing.T) {
	repository := &fakePlayerRepository{loaded: &Player{PlayerId: 42, PlayerInfo: &playermodel.PlayerInfo{PlayerName: "before"}}}
	registry := newPlayerRepositoryTestRegistry(t, repository)
	user := models.User{AccountId: "7_live", PlayerId: 42}
	prepared, err := registry.PrepareLogin(context.Background(), user, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	state := playersession.NewState()
	agent := &loginPlayerAgent{id: 1, writes: make(chan any, 4), loginStarted: make(chan struct{}), releaseLogin: make(chan struct{})}
	release := sync.OnceFunc(func() { close(agent.releaseLogin) })
	t.Cleanup(release)
	state.Register(agent)
	attempt, err := state.Begin(agent.id, 17)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		ok, err := prepared.Commit(context.Background(), attempt, user, &message.S2C_Login{LoginResult: 1})
		if err == nil && !ok {
			err = errors.New("commit rejected")
		}
		done <- err
	}()
	select {
	case <-agent.loginStarted:
	case <-time.After(time.Second):
		t.Fatal("login send did not start")
	}
	push := &message.S2C_Heart{}
	if err := registry.SendToClient(context.Background(), user.PlayerId, push); err != nil {
		t.Fatal(err)
	}
	other := &loginPlayerAgent{id: 2, writes: make(chan any, 4)}
	independent := make(chan error, 1)
	go func() { state.Register(other); _, err := state.Begin(other.id, 18); independent <- err }()
	select {
	case err := <-independent:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("network send held session lock")
	}
	release()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	first := <-agent.writes
	if reply, ok := first.(struct {
		msg any
		seq uint32
	}); !ok || reply.seq != 17 {
		t.Fatalf("first outgoing message = %#v", first)
	}
	if second := <-agent.writes; second != push {
		t.Fatalf("push order = %#v", second)
	}
	if _, _, err := registry.ModifyName(context.Background(), user.PlayerId, "live-unsaved"); err != nil {
		t.Fatal(err)
	}
	takeover, err := registry.PrepareLogin(context.Background(), user, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if takeover.player != prepared.player || takeover.owned || len(repository.loadIDs) != 1 || len(repository.saved) != 0 {
		t.Fatal("online preparation reloaded/stopped live Player")
	}
	if err := takeover.Abort(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot, err := registry.Snapshot(context.Background(), user.PlayerId)
	if err != nil || snapshot.PlayerInfo.PlayerName != "live-unsaved" {
		t.Fatalf("aborting reused Player changed state: %#v, %v", snapshot, err)
	}
}

type slowPlayerRepository struct {
	fakePlayerRepository
	started chan struct{}
	release chan struct{}
}

func (r *slowPlayerRepository) FindByID(id int64) (*Player, error) {
	close(r.started)
	<-r.release
	return r.fakePlayerRepository.FindByID(id)
}

func TestCanceledColdPreparationWaitsForInitializationAndDoesNotPersist(t *testing.T) {
	repository := &slowPlayerRepository{fakePlayerRepository: fakePlayerRepository{loaded: &Player{PlayerId: 42}}, started: make(chan struct{}), release: make(chan struct{})}
	registry := newPlayerRepositoryTestRegistry(t, repository)
	release := sync.OnceFunc(func() { close(repository.release) })
	t.Cleanup(release)
	user := models.User{AccountId: "7_cold", PlayerId: 42}
	state := playersession.NewState()
	agent := &lifecycleAgent{}
	state.Register(agent)
	attempt, _ := state.Begin(agent.ConnectionID(), 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type result struct {
		prepared *PreparedLogin
		err      error
	}
	done := make(chan result, 1)
	go func() { prepared, err := registry.PrepareLogin(ctx, user, nil, false); done <- result{prepared, err} }()
	select {
	case <-repository.started:
	case <-time.After(time.Second):
		t.Fatal("load did not start")
	}
	cancel()
	state.Disconnect(agent.ConnectionID())
	release()
	prepared := <-done
	if prepared.err != nil {
		t.Fatalf("accepted initialization lost its result: %v", prepared.err)
	}
	committed, err := prepared.prepared.Commit(context.Background(), attempt, user, &message.S2C_Login{LoginResult: 1})
	if err != nil || committed {
		t.Fatalf("canceled commit = %t, %v", committed, err)
	}
	if err := prepared.prepared.Abort(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.lookup(context.Background(), user.PlayerId); !errors.Is(err, actor.ErrActorStopped) {
		t.Fatalf("uncommitted Player still present: %v", err)
	}
	if len(repository.saved) != 0 || agent.UserData() != nil {
		t.Fatal("canceled preparation saved/bound Player")
	}
}
