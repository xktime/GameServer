package internal

import (
	"gameserver/common/session"
	"gameserver/core/chanrpc"
	"gameserver/core/gate"
	"net"
	"testing"
)

type closeEventAgent struct {
	userData any
}

var _ gate.Agent = (*closeEventAgent)(nil)

func (*closeEventAgent) WriteMsg(any)                {}
func (*closeEventAgent) WriteMsgWithSeq(any, uint32) {}
func (*closeEventAgent) LocalAddr() net.Addr         { return nil }
func (*closeEventAgent) RemoteAddr() net.Addr        { return nil }
func (*closeEventAgent) Close()                      {}
func (*closeEventAgent) Destroy()                    {}
func (a *closeEventAgent) UserData() any             { return a.userData }
func (a *closeEventAgent) SetUserData(data any)      { a.userData = data }

func TestCloseAgentBroadcastCarriesCapturedSessionSnapshot(t *testing.T) {
	previous := Dispatchers
	t.Cleanup(func() { Dispatchers = previous })

	dispatcher := chanrpc.NewServer(1)
	var received []interface{}
	dispatcher.Register("CloseAgent", func(args []interface{}) {
		received = args
	}, false)
	Dispatchers = []*chanrpc.Server{dispatcher}

	want := session.PlayerSession{ID: 7, AccountID: "1_open-id", PlayerID: 42}
	agent := &closeEventAgent{userData: session.PlayerSession{ID: 8, AccountID: "1_open-id", PlayerID: 42}}
	rpcCloseAgent([]interface{}{agent, want})
	dispatcher.Exec(<-dispatcher.ChanCall)

	if len(received) != 2 {
		t.Fatalf("CloseAgent 参数数 = %d，期望 2", len(received))
	}
	if received[0] != agent || received[1] != want {
		t.Fatalf("CloseAgent 参数 = %#v，期望 agent 与 %#v", received, want)
	}
}
