package playersession

import (
	"fmt"
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/common/session"
	"gameserver/core/gate"
	"net"
	"sync"
	"testing"
)

type testAgent struct {
	id     gate.ConnectionID
	mu     sync.Mutex
	data   any
	writes int
	closed bool
}

func (a *testAgent) ConnectionID() gate.ConnectionID   { return a.id }
func (a *testAgent) WriteMsg(any)                      { a.mu.Lock(); defer a.mu.Unlock(); a.writes++ }
func (a *testAgent) WriteMsgWithSeq(msg any, _ uint32) { a.WriteMsg(msg) }
func (*testAgent) LocalAddr() net.Addr                 { return nil }
func (*testAgent) RemoteAddr() net.Addr                { return nil }
func (a *testAgent) Close()                            { a.mu.Lock(); defer a.mu.Unlock(); a.closed = true }
func (a *testAgent) Destroy()                          { a.Close() }
func (a *testAgent) UserData() any                     { a.mu.Lock(); defer a.mu.Unlock(); return a.data }
func (a *testAgent) SetUserData(data any)              { a.mu.Lock(); defer a.mu.Unlock(); a.data = data }

func begin(t *testing.T, state *State, agent *testAgent) *Attempt {
	t.Helper()
	state.Register(agent)
	a, err := state.Begin(agent.id, 1)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestSupersededAttemptCannotReplyCloseOrBind(t *testing.T) {
	s := NewState()
	agent := &testAgent{id: 1}
	first := begin(t, s, agent)
	second := begin(t, s, agent)
	first.Fail()
	first.Reply(&message.S2C_Login{LoginResult: 1})
	if _, ok := first.Commit(models.User{AccountId: "u", PlayerId: 42}, &Slot{}); ok {
		t.Fatal("stale commit succeeded")
	}
	if agent.writes != 0 || agent.closed || agent.UserData() != nil {
		t.Fatal("stale attempt affected connection")
	}
	if !second.IsCurrent("") {
		t.Fatal("new attempt was invalidated by old failure")
	}
	s.Disconnect(agent.id)
	if _, ok := second.Commit(models.User{AccountId: "u", PlayerId: 42}, &Slot{}); ok {
		t.Fatal("disconnected commit succeeded")
	}
	third := begin(t, s, agent)
	if first.IsCurrent("") || second.IsCurrent("") || !third.IsCurrent("") {
		t.Fatal("reused connection accepted an old attempt")
	}
}

func TestSuccessfulLaterAttemptRemainsAuthoritativeAfterDisconnect(t *testing.T) {
	s := NewState()
	older := begin(t, s, &testAgent{id: 1})
	newAgent := &testAgent{id: 2}
	newer := begin(t, s, newAgent)
	user := models.User{AccountId: "u", PlayerId: 42}
	if _, ok := newer.Commit(user, &Slot{}); !ok {
		t.Fatal("newer commit failed")
	}
	current, _ := s.Current(user.AccountId)
	s.Disconnect(newAgent.id)
	if _, ok := s.Close(current.Snapshot); !ok {
		t.Fatal("current close failed")
	}
	if _, ok := older.Commit(user, &Slot{}); ok {
		t.Fatal("old authentication resurrected an offline session")
	}
	if _, ok := s.Current(user.AccountId); ok {
		t.Fatal("stale attempt created a session")
	}
}

func TestLaterPendingOrFailedAttemptDoesNotBlockEarlierConnection(t *testing.T) {
	for _, fail := range []bool{false, true} {
		s := NewState()
		older := begin(t, s, &testAgent{id: 1})
		newer := begin(t, s, &testAgent{id: 2})
		if fail {
			newer.Fail()
		}
		if _, ok := older.Commit(models.User{AccountId: "u", PlayerId: 42}, &Slot{}); !ok {
			t.Fatalf("earlier commit rejected, later failed=%t", fail)
		}
	}
}

func TestTakeoverIgnoresOldCloseAndCopiesUser(t *testing.T) {
	s := NewState()
	slot := &Slot{}
	first := begin(t, s, &testAgent{id: 1})
	user := models.User{AccountId: "u", PlayerId: 42, TotalLoginDays: 4}
	if _, ok := first.Commit(user, slot); !ok {
		t.Fatal("first commit failed")
	}
	old, _ := s.Current(user.AccountId)
	second := begin(t, s, &testAgent{id: 2})
	previous, ok := second.Commit(user, slot)
	if !ok || previous.Snapshot != old.Snapshot {
		t.Fatal("missing previous session")
	}
	if _, ok := s.Close(old.Snapshot); ok {
		t.Fatal("old close removed new session")
	}
	current, _ := s.Current(user.AccountId)
	if current.Snapshot.ID == old.Snapshot.ID || slot.Agent() != current.Agent || old.Agent.UserData() != nil {
		t.Fatal("session and send target diverged")
	}
	user.TotalLoginDays = 99
	if current.User.TotalLoginDays != 4 {
		t.Fatal("User state was shared by reference")
	}
}

func TestDisconnectAndCommitHaveOneOrdering(t *testing.T) {
	for i := 0; i < 100; i++ {
		s := NewState()
		agent := &testAgent{id: 1}
		a := begin(t, s, agent)
		start := make(chan struct{})
		committed := make(chan bool, 1)
		disconnected := make(chan struct{})
		go func() {
			<-start
			_, ok := a.Commit(models.User{AccountId: "u", PlayerId: 42}, &Slot{})
			committed <- ok
		}()
		go func() { <-start; s.Disconnect(agent.id); close(disconnected) }()
		close(start)
		won := <-committed
		<-disconnected
		current, active := s.Current("u")
		if active != won {
			t.Fatalf("active=%t commit=%t", active, won)
		}
		if won {
			if snapshot, ok := session.FromData(agent.UserData()); !ok || snapshot != current.Snapshot {
				t.Fatal("incomplete committed binding")
			}
			if _, ok := s.Close(current.Snapshot); !ok {
				t.Fatal("post-commit disconnect could not close session")
			}
		} else if agent.UserData() != nil {
			t.Fatal("canceled login bound an agent")
		}
	}
}

func TestLoginTimeoutObservationCannotExpireNewlyCommittedSession(t *testing.T) {
	s := NewState()
	agent := &testAgent{id: 1}
	a := begin(t, s, agent)
	unauthenticated := session.PlayerSession{}
	if _, ok := a.Commit(models.User{AccountId: "u", PlayerId: 42}, &Slot{}); !ok {
		t.Fatal("commit failed")
	}
	if s.Expire(agent.id, unauthenticated) {
		t.Fatal("old login timeout expired authenticated connection")
	}
	active, _ := s.Current("u")
	if !s.Expire(agent.id, active.Snapshot) {
		t.Fatal("current heartbeat timeout did not expire connection")
	}
}

func TestCompletedLoginsDoNotAccumulateOrderingHistory(t *testing.T) {
	s := NewState()
	for i := 1; i <= 100; i++ {
		agent := &testAgent{id: gate.ConnectionID(i)}
		a := begin(t, s, agent)
		user := models.User{AccountId: fmt.Sprintf("u-%d", i), PlayerId: int64(i)}
		if _, ok := a.Commit(user, &Slot{}); !ok {
			t.Fatal("commit failed")
		}
		current, _ := s.Current(user.AccountId)
		s.Disconnect(agent.id)
		if _, ok := s.Close(current.Snapshot); !ok {
			t.Fatal("close failed")
		}
	}
	if len(s.highestCommitted) != 0 {
		t.Fatalf("completed login history retained %d accounts", len(s.highestCommitted))
	}
}

func TestOrderingHistoryIsReleasedWhenEarlierAttemptsFinish(t *testing.T) {
	for _, finish := range []string{"fail", "disconnect", "supersede", "stale-check", "stale-commit"} {
		t.Run(finish, func(t *testing.T) {
			s := NewState()
			oldAgent := &testAgent{id: 1}
			older := begin(t, s, oldAgent)
			newer := begin(t, s, &testAgent{id: 2})
			user := models.User{AccountId: "u", PlayerId: 42}
			if _, ok := newer.Commit(user, &Slot{}); !ok {
				t.Fatal("newer commit failed")
			}
			active, _ := s.Current(user.AccountId)
			s.Disconnect(active.Agent.ConnectionID())
			s.Close(active.Snapshot)
			later := begin(t, s, &testAgent{id: 3})
			if len(s.highestCommitted) != 1 {
				t.Fatal("ordering history was dropped while an earlier attempt was pending")
			}
			switch finish {
			case "fail":
				older.Fail()
			case "disconnect":
				s.Disconnect(oldAgent.id)
			case "supersede":
				begin(t, s, oldAgent)
			case "stale-check":
				if older.IsCurrent(user.AccountId) {
					t.Fatal("older attempt remained current after later success")
				}
			case "stale-commit":
				if _, ok := older.Commit(user, &Slot{}); ok {
					t.Fatal("older attempt resurrected the closed session")
				}
			}
			if len(s.highestCommitted) != 0 {
				t.Fatalf("finished earlier attempt retained %d accounts", len(s.highestCommitted))
			}
			if !later.IsCurrent(user.AccountId) {
				t.Fatal("reclaiming history invalidated a later pending attempt")
			}
			if _, ok := older.Commit(user, &Slot{}); ok {
				t.Fatal("reclaiming history made a finished attempt valid again")
			}
		})
	}
}

func TestOrderingHistorySurvivesFinishingANewerPendingAttempt(t *testing.T) {
	s := NewState()
	older := begin(t, s, &testAgent{id: 1})
	middle := begin(t, s, &testAgent{id: 2})
	newer := begin(t, s, &testAgent{id: 3})
	user := models.User{AccountId: "u", PlayerId: 42}
	if _, ok := newer.Commit(user, &Slot{}); !ok {
		t.Fatal("newer commit failed")
	}
	middle.Fail()
	if len(s.highestCommitted) != 1 {
		t.Fatal("finishing a newer pending attempt discarded required ordering history")
	}
	if _, ok := older.Commit(user, &Slot{}); ok {
		t.Fatal("older attempt replaced the newer session")
	}
	if len(s.pendingAttempts) != 0 || len(s.highestCommitted) != 0 {
		t.Fatal("terminal attempts retained pending state or ordering history")
	}
}
