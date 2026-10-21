package managers

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	provisionmanager "gameserver/modules/game/internal/managers/provision"
	gameprovision "gameserver/modules/game/internal/provision"
	"gameserver/modules/game/playersession"
	"reflect"
	"sync"
	"testing"
	"time"
)

func settleUser(t *testing.T, manager *UserManager) {
	t.Helper()
	if _, err := actor.CallUntilDone(context.Background(), manager.Ref(), func(actor.Context) (struct{}, error) { return struct{}{}, nil }); err != nil {
		t.Fatal(err)
	}
}

func waitLoginSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal("login did not reach barrier")
	}
}

func TestTakeoverKeepsLivePlayerTeamAndRoomAndRejectsOldClose(t *testing.T) {
	manager := newUserManagerRepositoryTestHarness(t, &fakeUserRepository{}, provisionmanager.NewMemoryRepository())
	offline := 0
	manager.SetOfflineNotifier(func(int64) bool { offline++; return true })
	first := &userManagerAgent{}
	if response := testUserLogin(t, manager, first, "takeover", 7, message.LoginType_None); response.LoginResult != 1 {
		t.Fatal(response)
	}
	old, _ := manager.sessions.Current("7_takeover")
	if _, _, err := manager.players.ModifyName(context.Background(), old.User.PlayerId, "unsaved-name"); err != nil {
		t.Fatal(err)
	}
	before, err := manager.players.Snapshot(context.Background(), old.User.PlayerId)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.teams.SetRoomProjection(context.Background(), before.TeamID, "live-room"); err != nil {
		t.Fatal(err)
	}
	teamBefore, err := manager.teams.Snapshot(context.Background(), before.TeamID)
	if err != nil {
		t.Fatal(err)
	}
	second := &userManagerAgent{}
	if response := testUserLogin(t, manager, second, "takeover", 7, message.LoginType_None); response.LoginResult != 1 {
		t.Fatal(response)
	}
	current, _ := manager.sessions.Current("7_takeover")
	if current.Snapshot == old.Snapshot || !first.closed || first.UserData() != nil {
		t.Fatal("old connection was not replaced")
	}
	manager.ClosePlayerSession(old.Snapshot)
	manager.ClosePlayerSession(old.Snapshot)
	settleUser(t, manager)
	after, err := manager.players.Snapshot(context.Background(), old.User.PlayerId)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("live Player changed: before=%#v after=%#v err=%v", before, after, err)
	}
	teamAfter, err := manager.teams.Snapshot(context.Background(), before.TeamID)
	if err != nil || !reflect.DeepEqual(teamBefore, teamAfter) {
		t.Fatalf("Team/Room changed: before=%#v after=%#v err=%v", teamBefore, teamAfter, err)
	}
	if offline != 0 || second.closed || second.UserData() != current.Snapshot {
		t.Fatal("old close affected replacement Session")
	}
}

func TestCanceledTakeoverKeepsOldSessionDuringSlowPreparation(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		t.Run(map[bool]string{false: "new-attempt", true: "disconnect"}[disconnect], func(t *testing.T) {
			manager := newUserManagerRepositoryTestHarness(t, &fakeUserRepository{}, provisionmanager.NewMemoryRepository())
			oldAgent := &userManagerAgent{}
			if response := testUserLogin(t, manager, oldAgent, "slow", 7, message.LoginType_None); response.LoginResult != 1 {
				t.Fatal(response)
			}
			old, _ := manager.sessions.Current("7_slow")
			playerBlocked, releasePlayer := make(chan struct{}), make(chan struct{})
			release := sync.OnceFunc(func() { close(releasePlayer) })
			t.Cleanup(release)
			oldAgent.beforeWrite = sync.OnceFunc(func() { close(playerBlocked); <-releasePlayer })
			if err := manager.players.SendToClient(context.Background(), old.User.PlayerId, &message.S2C_Heart{}); err != nil {
				t.Fatal(err)
			}
			waitLoginSignal(t, playerBlocked)
			newAgent := &userManagerAgent{}
			attempt := beginUserLogin(t, manager, newAgent, 9)
			preparing := make(chan struct{})
			done := make(chan error, 1)
			go func() {
				_, err := actor.CallUntilDone(context.Background(), manager.Ref(), func(execution actor.Context) (struct{}, error) {
					close(preparing)
					return struct{}{}, manager.doUserLogin(execution, attempt, "slow", 7, message.LoginType_None)
				})
				done <- err
			}()
			waitLoginSignal(t, preparing)
			if oldAgent.closed || oldAgent.UserData() != old.Snapshot {
				t.Fatal("preparation kicked old session")
			}
			if disconnect {
				manager.sessions.Disconnect(newAgent.id)
			} else {
				_ = beginUserLogin(t, manager, newAgent, 10)
			}
			release()
			if err := <-done; !errors.Is(err, playersession.ErrStaleAttempt) {
				t.Fatalf("canceled login = %v", err)
			}
			current, _ := manager.sessions.Current("7_slow")
			if current.Snapshot != old.Snapshot || oldAgent.closed || len(newAgent.writes) != 0 || newAgent.UserData() != nil {
				t.Fatal("canceled takeover changed a session")
			}
		})
	}
}

func TestFailedTakeoverLeavesOldSessionOnline(t *testing.T) {
	repository := &fakeUserRepository{}
	manager := newUserManagerRepositoryTestHarness(t, repository, provisionmanager.NewMemoryRepository())
	oldAgent := &userManagerAgent{}
	_ = testUserLogin(t, manager, oldAgent, "failure", 7, message.LoginType_None)
	old, _ := manager.sessions.Current("7_failure")
	if err := manager.players.Stop(context.Background(), old.User.PlayerId); err != nil {
		t.Fatal(err)
	}
	newAgent := &userManagerAgent{}
	if response := testUserLogin(t, manager, newAgent, "failure", 7, message.LoginType_None); response.LoginResult != -1 {
		t.Fatal(response)
	}
	current, _ := manager.sessions.Current("7_failure")
	if current.Snapshot != old.Snapshot || oldAgent.closed || !newAgent.closed {
		t.Fatal("failed preparation replaced old session")
	}
}

func TestTakeoverUsesCurrentUserWithoutRepositoryLookup(t *testing.T) {
	repository := &fakeUserRepository{}
	manager := newUserManagerRepositoryTestHarness(t, repository, provisionmanager.NewMemoryRepository())
	oldAgent := &userManagerAgent{}
	_ = testUserLogin(t, manager, oldAgent, "memory", 7, message.LoginType_None)
	old, _ := manager.sessions.Current("7_memory")
	repository.findErr = errors.New("database unavailable")
	newAgent := &userManagerAgent{}
	if response := testUserLogin(t, manager, newAgent, "memory", 7, message.LoginType_None); response.LoginResult != 1 {
		t.Fatalf("takeover required a database lookup: %#v", response)
	}
	current, ok := manager.sessions.Current("7_memory")
	if !ok || current.Snapshot == old.Snapshot || current.User.PlayerId != old.User.PlayerId || !oldAgent.closed || newAgent.closed {
		t.Fatal("takeover did not preserve the Player and replace its connection")
	}
	if len(repository.findCalls) != 1 {
		t.Fatalf("login lookups = %d, want only the initial lookup", len(repository.findCalls))
	}
}

func TestEarlierLoginCannotReturnAfterLaterSuccessGoesOffline(t *testing.T) {
	manager := newUserManagerRepositoryTestHarness(t, &fakeUserRepository{}, provisionmanager.NewMemoryRepository())
	offline := 0
	manager.SetOfflineNotifier(func(int64) bool { offline++; return true })
	oldAgent := &userManagerAgent{}
	older := beginUserLogin(t, manager, oldAgent, 1)
	newAgent := &userManagerAgent{}
	_ = testUserLogin(t, manager, newAgent, "ordered", 7, message.LoginType_None)
	newSession, _ := manager.sessions.Current("7_ordered")
	manager.sessions.Disconnect(newAgent.id)
	manager.ClosePlayerSession(newSession.Snapshot)
	settleUser(t, manager)
	if err := manager.UserLogin(older, "ordered", 7, message.LoginType_None); !errors.Is(err, playersession.ErrStaleAttempt) {
		t.Fatalf("late older login = %v", err)
	}
	if _, ok := manager.sessions.Current("7_ordered"); ok {
		t.Fatal("older login resurrected session")
	}
	if offline != 1 || len(oldAgent.writes) != 0 || oldAgent.UserData() != nil {
		t.Fatal("wrong offline count or stale success")
	}
}

type savedIntentBarrier struct {
	provisionmanager.Repository
	once    sync.Once
	saved   chan struct{}
	release chan struct{}
}

func (r *savedIntentBarrier) Save(intent *gameprovision.Intent) error {
	if err := r.Repository.Save(intent); err != nil {
		return err
	}
	r.once.Do(func() { close(r.saved); <-r.release })
	return nil
}

func TestCanceledFirstLoginPreservesDurableIntentForRetry(t *testing.T) {
	intents := &savedIntentBarrier{Repository: provisionmanager.NewMemoryRepository(), saved: make(chan struct{}), release: make(chan struct{})}
	manager := newUserManagerRepositoryTestHarness(t, &fakeUserRepository{}, intents)
	release := sync.OnceFunc(func() { close(intents.release) })
	t.Cleanup(release)
	agent := &userManagerAgent{}
	attempt := beginUserLogin(t, manager, agent, 1)
	done := make(chan error, 1)
	go func() { done <- manager.UserLogin(attempt, "intent", 7, message.LoginType_None) }()
	waitLoginSignal(t, intents.saved)
	manager.sessions.Disconnect(agent.id)
	release()
	if err := <-done; !errors.Is(err, playersession.ErrStaleAttempt) {
		t.Fatal(err)
	}
	intent, err := intents.FindByAccount("7_intent")
	if err != nil || intent == nil {
		t.Fatalf("durable intent lost: %v", err)
	}
	if _, err := manager.players.Snapshot(context.Background(), intent.Player.PlayerID); !errors.Is(err, actor.ErrActorStopped) {
		t.Fatalf("uncommitted Player remained: %v", err)
	}
	next := &userManagerAgent{}
	if response := testUserLogin(t, manager, next, "intent", 7, message.LoginType_None); response.LoginResult != 1 {
		t.Fatal(response)
	}
	active, _ := manager.sessions.Current("7_intent")
	if active.User.PlayerId != intent.Player.PlayerID {
		t.Fatal("retry allocated a different Player")
	}
}
