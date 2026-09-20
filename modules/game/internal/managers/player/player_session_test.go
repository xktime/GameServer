package player

import (
	"context"
	"gameserver/common/session"
	"gameserver/core/gate"
	"net"
	"testing"
)

type lifecycleAgent struct {
	closed   bool
	userData any
}

var _ gate.Agent = (*lifecycleAgent)(nil)

func (*lifecycleAgent) WriteMsg(any)                {}
func (*lifecycleAgent) WriteMsgWithSeq(any, uint32) {}
func (*lifecycleAgent) LocalAddr() net.Addr         { return nil }
func (*lifecycleAgent) RemoteAddr() net.Addr        { return nil }
func (a *lifecycleAgent) Close()                    { a.closed = true }
func (*lifecycleAgent) Destroy()                    {}
func (a *lifecycleAgent) UserData() any             { return a.userData }
func (a *lifecycleAgent) SetUserData(data any)      { a.userData = data }

func TestPlayerStopDoesNotOwnSessionConnection(t *testing.T) {
	snapshot := session.PlayerSession{ID: 7, AccountID: "1_open-id", PlayerID: 42}
	agent := &lifecycleAgent{userData: snapshot}
	player := &Player{PlayerId: snapshot.PlayerID, agent: agent, repository: &fakePlayerRepository{}}

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
