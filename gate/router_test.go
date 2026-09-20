package gate

import (
	"gameserver/common/msg/message"
	"gameserver/common/session"
	coregate "gameserver/core/gate"
	"net"
	"testing"
)

type routeAgent struct {
	userData any
}

var _ coregate.Agent = (*routeAgent)(nil)

func (*routeAgent) WriteMsg(any)                {}
func (*routeAgent) WriteMsgWithSeq(any, uint32) {}
func (*routeAgent) LocalAddr() net.Addr         { return nil }
func (*routeAgent) RemoteAddr() net.Addr        { return nil }
func (*routeAgent) Close()                      {}
func (*routeAgent) Destroy()                    {}
func (a *routeAgent) UserData() any             { return a.userData }
func (a *routeAgent) SetUserData(data any)      { a.userData = data }

func TestPlayerSessionRouteFilter(t *testing.T) {
	unauthenticated := &routeAgent{}
	if err := requirePlayerSession(&message.C2S_Login{}, unauthenticated); err != nil {
		t.Fatalf("登录消息被拒绝: %v", err)
	}
	if err := requirePlayerSession(&message.C2S_Heart{}, unauthenticated); err != nil {
		t.Fatalf("心跳消息被拒绝: %v", err)
	}
	if err := requirePlayerSession(&message.C2S_CheckName{}, unauthenticated); err == nil {
		t.Fatal("未认证的业务消息未被拒绝")
	}

	authenticated := &routeAgent{userData: session.PlayerSession{ID: 7, AccountID: "1_open-id", PlayerID: 42}}
	if err := requirePlayerSession(&message.C2S_CheckName{}, authenticated); err != nil {
		t.Fatalf("已认证的业务消息被拒绝: %v", err)
	}
}
