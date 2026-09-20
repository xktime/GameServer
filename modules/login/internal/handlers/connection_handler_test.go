package handlers

import (
	"errors"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"net"
	"testing"
)

type handlerConnectionAgent struct {
	id        gate.ConnectionID
	closed    bool
	writes    []any
	sequences []uint32
}

var _ gate.ConnectionAgent = (*handlerConnectionAgent)(nil)

func (a *handlerConnectionAgent) WriteMsg(msg any) { a.writes = append(a.writes, msg) }
func (a *handlerConnectionAgent) WriteMsgWithSeq(msg any, seq uint32) {
	a.writes = append(a.writes, msg)
	a.sequences = append(a.sequences, seq)
}
func (*handlerConnectionAgent) LocalAddr() net.Addr               { return nil }
func (*handlerConnectionAgent) RemoteAddr() net.Addr              { return nil }
func (a *handlerConnectionAgent) ConnectionID() gate.ConnectionID { return a.id }
func (a *handlerConnectionAgent) Close()                          { a.closed = true }
func (*handlerConnectionAgent) Destroy()                          {}
func (*handlerConnectionAgent) UserData() any                     { return nil }
func (*handlerConnectionAgent) SetUserData(any)                   {}

type handlerConnections struct {
	heartbeats []gate.ConnectionID
}

func (*handlerConnections) Register(gate.ConnectionAgent) {}
func (c *handlerConnections) UpdateHeartbeat(id gate.ConnectionID) {
	c.heartbeats = append(c.heartbeats, id)
}
func (*handlerConnections) RemoveClient(gate.ConnectionID) {}

type asyncFixedLoginManager struct {
	submitted chan uint32
	submitErr error
}

func (m asyncFixedLoginManager) SubmitLogin(_ *message.C2S_Login, _ gate.ConnectionAgent, sequence uint32) error {
	if m.submitErr != nil {
		return m.submitErr
	}
	m.submitted <- sequence
	return nil
}

func (asyncFixedLoginManager) Close() {}

func TestLoginHandlerSubmitsLogin(t *testing.T) {
	agent := &handlerConnectionAgent{id: 7}
	login := asyncFixedLoginManager{submitted: make(chan uint32, 1)}

	C2S_LoginHandler(
		login,
		[]interface{}{&message.C2S_Login{}, agent, uint32(11)},
	)

	if len(agent.writes) != 0 {
		t.Fatalf("异步登录提交后同步写回了响应: %v", agent.writes)
	}
	select {
	case sequence := <-login.submitted:
		if sequence != 11 {
			t.Fatalf("异步登录序号 = %d，期望 11", sequence)
		}
	default:
		t.Fatal("异步登录未提交到 Worker Manager")
	}
}

func TestLoginHandlerWritesFailureWhenSubmitFails(t *testing.T) {
	agent := &handlerConnectionAgent{id: 7}
	login := asyncFixedLoginManager{submitErr: errors.New("queue full")}

	C2S_LoginHandler(
		login,
		[]interface{}{&message.C2S_Login{}, agent, uint32(11)},
	)

	if len(agent.writes) != 1 || agent.sequences[0] != 11 {
		t.Fatalf("提交失败响应 = %v，序号 = %v，期望一条序号为 11 的响应", agent.writes, agent.sequences)
	}
	response, ok := agent.writes[0].(*message.S2C_Login)
	if !ok || response.LoginResult != -1 {
		t.Fatalf("提交失败响应 = %#v，期望 LoginResult=-1", agent.writes[0])
	}
	if !agent.closed {
		t.Fatal("提交失败后连接应关闭")
	}
}

func TestHeartHandlerRefreshesConnectionByID(t *testing.T) {
	agent := &handlerConnectionAgent{id: 7}
	connections := &handlerConnections{}
	C2S_HeartHandler(connections, []interface{}{&message.C2S_Heart{}, agent, uint32(11)})

	if len(connections.heartbeats) != 1 || connections.heartbeats[0] != agent.ConnectionID() {
		t.Fatalf("心跳刷新 = %v，期望 [%d]", connections.heartbeats, agent.ConnectionID())
	}
}
