package internal

import (
	"gameserver/core/gate"
	"net"
	"testing"
)

type rpcConnectionAgent struct {
	id gate.ConnectionID
}

var _ gate.ConnectionAgent = (*rpcConnectionAgent)(nil)

func (*rpcConnectionAgent) WriteMsg(any)                      {}
func (*rpcConnectionAgent) WriteMsgWithSeq(any, uint32)       {}
func (*rpcConnectionAgent) LocalAddr() net.Addr               { return nil }
func (*rpcConnectionAgent) RemoteAddr() net.Addr              { return nil }
func (a *rpcConnectionAgent) ConnectionID() gate.ConnectionID { return a.id }
func (*rpcConnectionAgent) Close()                            {}
func (*rpcConnectionAgent) Destroy()                          {}
func (*rpcConnectionAgent) UserData() any                     { return nil }
func (*rpcConnectionAgent) SetUserData(any)                   {}

type recordingConnections struct {
	registered []gate.ConnectionAgent
	heartbeats []gate.ConnectionID
	removed    []gate.ConnectionID
}

func (c *recordingConnections) Register(agent gate.ConnectionAgent) {
	c.registered = append(c.registered, agent)
}

func (c *recordingConnections) UpdateHeartbeat(id gate.ConnectionID) {
	c.heartbeats = append(c.heartbeats, id)
}

func (c *recordingConnections) RemoveClient(id gate.ConnectionID) {
	c.removed = append(c.removed, id)
}

func TestAgentRPCTracksConnectionLifecycleByID(t *testing.T) {
	connections := &recordingConnections{}
	agent := &rpcConnectionAgent{id: 7}

	rpcNewAgent(connections, []interface{}{agent})
	rpcCloseAgent(connections, []interface{}{agent})

	if len(connections.registered) != 1 || connections.registered[0] != agent {
		t.Fatalf("登记连接 = %#v，期望 [%p]", connections.registered, agent)
	}
	if len(connections.removed) != 1 || connections.removed[0] != agent.ConnectionID() {
		t.Fatalf("移除 ConnectionID = %v，期望 [%d]", connections.removed, agent.ConnectionID())
	}
}
