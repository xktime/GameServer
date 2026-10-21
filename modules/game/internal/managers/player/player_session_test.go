package player

import (
	"context"
	"gameserver/common/models"
	"gameserver/common/session"
	"gameserver/core/gate"
	"gameserver/modules/game/playersession"
	"net"
	"testing"
)

type lifecycleAgent struct {
	closed   bool
	userData any
}

var _ gate.Agent = (*lifecycleAgent)(nil)

func (*lifecycleAgent) WriteMsg(any)                    {}
func (*lifecycleAgent) WriteMsgWithSeq(any, uint32)     {}
func (*lifecycleAgent) LocalAddr() net.Addr             { return nil }
func (*lifecycleAgent) RemoteAddr() net.Addr            { return nil }
func (*lifecycleAgent) ConnectionID() gate.ConnectionID { return 1 }
func (a *lifecycleAgent) Close()                        { a.closed = true }
func (*lifecycleAgent) Destroy()                        {}
func (a *lifecycleAgent) UserData() any                 { return a.userData }
func (a *lifecycleAgent) SetUserData(data any)          { a.userData = data }

func TestPlayerStopDoesNotOwnSessionConnection(t *testing.T) {
	agent := &lifecycleAgent{}
	player := &Player{PlayerId: 42, repository: &fakePlayerRepository{}}
	state := playersession.NewState()
	state.Register(agent)
	attempt, err := state.Begin(agent.ConnectionID(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := attempt.Commit(models.User{AccountId: "1_open-id", PlayerId: 42}, &player.connection); !ok {
		t.Fatal("commit failed")
	}
	snapshot, ok := session.FromData(agent.UserData())
	if !ok {
		t.Fatal("missing session")
	}

	if err := player.OnStop(context.Background()); err != nil {
		t.Fatalf("停止 Player: %v", err)
	}
	if agent.closed {
		t.Fatal("Player.OnStop 不应关闭 Player Session 的连接")
	}
	if agent.UserData() != snapshot {
		t.Fatalf("Player.OnStop 改写了会话快照: %#v", agent.UserData())
	}
}
