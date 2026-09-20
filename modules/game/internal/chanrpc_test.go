package internal

import (
	"gameserver/common/session"
	"testing"
)

type recordingSessionCloser struct {
	closed []session.PlayerSession
}

func (r *recordingSessionCloser) ClosePlayerSession(snapshot session.PlayerSession) {
	r.closed = append(r.closed, snapshot)
}

func TestCloseAgentUsesBroadcastSnapshot(t *testing.T) {
	closer := &recordingSessionCloser{}
	want := session.PlayerSession{ID: 7, AccountID: "1_open-id", PlayerID: 42}

	rpcCloseAgent(closer, []interface{}{nil, want})

	if len(closer.closed) != 1 || closer.closed[0] != want {
		t.Fatalf("关闭的 Player Session = %#v，期望 %#v", closer.closed, want)
	}
}
