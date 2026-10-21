// Package playersession owns login attempts and the current connection of a Player.
package playersession

import (
	"errors"
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/common/session"
	"gameserver/core/gate"
	"sync"
)

var (
	ErrDisconnected  = errors.New("login: connection is not registered")
	ErrAuthenticated = errors.New("login: connection already has a Player Session")
	ErrStaleAttempt  = errors.New("login: attempt is no longer current")
)

type outcome uint8

const (
	pending outcome = iota
	committed
	aborted
)

type connection struct {
	agent         gate.ConnectionAgent
	attempt       *Attempt
	live          bool
	authenticated bool
}

// Attempt is issued only by State.Begin; its terminal result is decided once.
type Attempt struct {
	state      *State
	connection *connection
	order      uint64
	sequence   uint32
	result     outcome
	replied    bool
}

// Active contains a value copy of User, never shared mutable User fields.
type Active struct {
	Snapshot session.PlayerSession
	User     models.User
	Agent    gate.ConnectionAgent
}

// Slot is read and changed only by its Player Actor. Commit changes it together
// with the authoritative session index; sends stay on the existing Actor queue.
type Slot struct {
	active Active
}

func (s *Slot) Agent() gate.Agent { return s.active.Agent }

type State struct {
	mu              sync.Mutex
	nextAttempt     uint64
	nextSession     uint64
	connections     map[gate.ConnectionID]*connection
	active          map[string]Active
	pendingAttempts map[uint64]struct{}
	oldestPending   uint64
	// Retained only while an earlier pending attempt could still return.
	highestCommitted map[string]uint64
}

func NewState() *State {
	return &State{
		connections:      make(map[gate.ConnectionID]*connection),
		active:           make(map[string]Active),
		pendingAttempts:  make(map[uint64]struct{}),
		highestCommitted: make(map[string]uint64),
	}
}

func (s *State) Register(agent gate.ConnectionAgent) {
	if agent == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.connections[agent.ConnectionID()]; !exists {
		s.connections[agent.ConnectionID()] = &connection{agent: agent, live: true}
	}
}

func (s *State) Begin(connectionID gate.ConnectionID, sequence uint32) (*Attempt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.connections[connectionID]
	if c == nil || !c.live {
		return nil, ErrDisconnected
	}
	if c.authenticated {
		return nil, ErrAuthenticated
	}
	if c.attempt != nil && c.attempt.result == pending {
		c.attempt.finishLocked(aborted)
	}
	s.nextAttempt++
	a := &Attempt{state: s, connection: c, order: s.nextAttempt, sequence: sequence}
	if len(s.pendingAttempts) == 0 {
		s.oldestPending = a.order
	}
	s.pendingAttempts[a.order] = struct{}{}
	c.attempt = a
	return a, nil
}

// Disconnect is a short, idempotent memory operation, safe in Gate's close hook.
// The active Session remains until UserManager confirms its exact snapshot.
func (s *State) Disconnect(connectionID gate.ConnectionID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disconnectLocked(connectionID)
}

// Expire only closes the Session observed by the heartbeat scan. A successful
// commit between that observation and expiry gets its authenticated deadline.
func (s *State) Expire(connectionID gate.ConnectionID, observed session.PlayerSession) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.connections[connectionID]; c != nil {
		current, _ := session.FromData(c.agent.UserData())
		if current != observed {
			return false
		}
	}
	s.disconnectLocked(connectionID)
	return true
}

func (s *State) disconnectLocked(connectionID gate.ConnectionID) {
	if c := s.connections[connectionID]; c != nil {
		c.live = false
		if c.attempt != nil && c.attempt.result == pending {
			c.attempt.finishLocked(aborted)
		}
		delete(s.connections, connectionID)
	}
}

func (a *Attempt) validLocked(accountID string) bool {
	return a.result == pending && a.connection.live && a.connection.attempt == a &&
		(accountID == "" || a.order > a.state.highestCommitted[accountID])
}

func (a *Attempt) finishLocked(result outcome) {
	s := a.state
	a.result = result
	delete(s.pendingAttempts, a.order)
	if a.order != s.oldestPending {
		return
	}
	s.oldestPending = s.nextAttempt + 1
	for order := range s.pendingAttempts {
		s.oldestPending = min(s.oldestPending, order)
	}
	for accountID, order := range s.highestCommitted {
		if order < s.oldestPending {
			delete(s.highestCommitted, accountID)
		}
	}
}

func (a *Attempt) IsCurrent(accountID string) bool {
	if a == nil {
		return false
	}
	a.state.mu.Lock()
	defer a.state.mu.Unlock()
	current := a.validLocked(accountID)
	if !current && a.result == pending {
		a.finishLocked(aborted)
	}
	return current
}

// Commit must run on the Player Actor owning slot. It contains no network I/O
// or Actor calls. The production Agent's SetUserData only changes memory.
func (a *Attempt) Commit(user models.User, slot *Slot) (Active, bool) {
	if a == nil || slot == nil || user.AccountId == "" || user.PlayerId == 0 {
		return Active{}, false
	}
	s := a.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if !a.validLocked(user.AccountId) {
		if a.result == pending {
			a.finishLocked(aborted)
		}
		return Active{}, false
	}
	previous := s.active[user.AccountId]
	s.nextSession++
	active := Active{
		Snapshot: session.PlayerSession{ID: s.nextSession, AccountID: user.AccountId, PlayerID: user.PlayerId},
		User:     user,
		Agent:    a.connection.agent,
	}
	if previous.Agent != nil {
		previous.Agent.SetUserData(nil)
		if old := s.connections[previous.Agent.ConnectionID()]; old != nil {
			old.live = false
			delete(s.connections, previous.Agent.ConnectionID())
		}
	}
	active.Agent.SetUserData(active.Snapshot)
	s.active[user.AccountId] = active
	s.highestCommitted[user.AccountId] = a.order
	slot.active = active
	a.connection.authenticated = true
	a.finishLocked(committed)
	return previous, true
}

// Reply is called exactly once by the Player task which won Commit, outside
// the state lock and before that Actor accepts its next outgoing message.
func (a *Attempt) Reply(response *message.S2C_Login) {
	if a == nil || response == nil {
		return
	}
	a.state.mu.Lock()
	if a.result != committed || a.replied {
		a.state.mu.Unlock()
		return
	}
	a.replied = true
	a.state.mu.Unlock()
	a.connection.agent.WriteMsgWithSeq(response, a.sequence)
}

// Fail owns both the failure reply and close. Superseded/committed attempts do
// nothing, so late failures cannot close a newer attempt on the same connection.
func (a *Attempt) Fail() {
	if a == nil {
		return
	}
	s := a.state
	s.mu.Lock()
	if !a.validLocked("") {
		s.mu.Unlock()
		return
	}
	a.finishLocked(aborted)
	a.connection.live = false
	delete(s.connections, a.connection.agent.ConnectionID())
	s.mu.Unlock()
	a.connection.agent.WriteMsgWithSeq(&message.S2C_Login{LoginResult: -1}, a.sequence)
	a.connection.agent.Close()
}

func (s *State) Current(accountID string) (Active, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	active, ok := s.active[accountID]
	return active, ok
}

func (s *State) Close(snapshot session.PlayerSession) (Active, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	active, ok := s.active[snapshot.AccountID]
	if !ok || active.Snapshot != snapshot {
		return Active{}, false
	}
	delete(s.active, snapshot.AccountID)
	if c := s.connections[active.Agent.ConnectionID()]; c != nil {
		c.live = false
		delete(s.connections, active.Agent.ConnectionID())
	}
	active.Agent.SetUserData(nil)
	return active, true
}

func (s *State) All() []Active {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Active, 0, len(s.active))
	for _, active := range s.active {
		result = append(result, active)
	}
	return result
}
