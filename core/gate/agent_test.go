package gate

import (
	"gameserver/common/session"
	"sync"
	"testing"
)

func TestAgentKeepsCloseSnapshotAfterSessionUnbound(t *testing.T) {
	agent := &agent{}
	want := session.PlayerSession{ID: 7, AccountID: "1_open-id", PlayerID: 42}
	agent.SetUserData(want)
	agent.SetUserData(nil)

	if agent.UserData() != nil {
		t.Fatalf("解绑后的 UserData = %#v，期望 nil", agent.UserData())
	}
	if got := agent.closeSnapshot(); got != want {
		t.Fatalf("CloseAgent 快照 = %#v，期望 %#v", got, want)
	}
}

func TestAgentUserDataSupportsConcurrentSnapshotAccess(t *testing.T) {
	agent := &agent{}
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func(value int) {
			defer workers.Done()
			for j := 0; j < 100; j++ {
				agent.SetUserData(value)
				_ = agent.UserData()
			}
		}(i)
	}
	workers.Wait()
}

func TestGateAssignsUniqueConnectionIDs(t *testing.T) {
	server := &Gate{}
	first := server.newAgent(nil)
	second := server.newAgent(nil)

	var _ ConnectionAgent = first
	if first.ConnectionID() == 0 || second.ConnectionID() == 0 {
		t.Fatalf("ConnectionID 必须非零: first=%d second=%d", first.ConnectionID(), second.ConnectionID())
	}
	if first.ConnectionID() == second.ConnectionID() {
		t.Fatalf("连接复用了 ConnectionID %d", first.ConnectionID())
	}
}
