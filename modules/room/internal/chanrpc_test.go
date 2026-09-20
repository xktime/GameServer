package internal

import (
	"gameserver/common/session"
	"testing"
)

type recordingOfflineHandler struct {
	playerIDs []int64
}

func (h *recordingOfflineHandler) PlayerOffline(playerID int64) bool {
	h.playerIDs = append(h.playerIDs, playerID)
	return true
}

func TestCloseAgentUsesCapturedSessionSnapshot(t *testing.T) {
	handler := &recordingOfflineHandler{}
	want := session.PlayerSession{ID: 7, AccountID: "1_open-id", PlayerID: 42}

	rpcCloseAgent(handler, []interface{}{nil, want})

	if len(handler.playerIDs) != 1 || handler.playerIDs[0] != want.PlayerID {
		t.Fatalf("下线 Player ID = %v，期望 [%d]", handler.playerIDs, want.PlayerID)
	}
}
