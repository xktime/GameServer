package managers

import (
	"gameserver/common/models"
	"testing"
)

func TestPlayerSessionsIgnoreStaleCloseAfterRelogin(t *testing.T) {
	sessions := newPlayerSessions()
	user := &models.User{AccountId: "1_open-id", PlayerId: 42}

	first, err := sessions.open(user, nil)
	if err != nil {
		t.Fatalf("打开首个会话: %v", err)
	}
	closed, ok := sessions.close(first.snapshot)
	if !ok || closed.snapshot != first.snapshot {
		t.Fatalf("关闭首个会话 = %#v, %t", closed, ok)
	}

	second, err := sessions.open(user, nil)
	if err != nil {
		t.Fatalf("打开重登录会话: %v", err)
	}
	if first.snapshot.ID == second.snapshot.ID {
		t.Fatalf("重登录复用了会话 ID %d", first.snapshot.ID)
	}

	if stale, ok := sessions.close(first.snapshot); ok {
		t.Fatalf("迟到的旧断线关闭了会话: %#v", stale)
	}
	current, ok := sessions.current(user.AccountId)
	if !ok || current.snapshot != second.snapshot {
		t.Fatalf("当前会话 = %#v, %t，期望 %#v, true", current, ok, second)
	}
}

func TestPlayerSessionsRejectSecondOpenForSameAccount(t *testing.T) {
	sessions := newPlayerSessions()
	user := &models.User{AccountId: "1_open-id", PlayerId: 42}
	if _, err := sessions.open(user, nil); err != nil {
		t.Fatalf("打开首个会话: %v", err)
	}
	if _, err := sessions.open(user, nil); err == nil {
		t.Fatal("同一账号已有会话时不应静默覆盖")
	}
}
