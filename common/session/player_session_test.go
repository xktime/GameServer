package session

import (
	"gameserver/common/models"
	"testing"
)

func TestFromDataAcceptsOnlyPlayerSessionSnapshots(t *testing.T) {
	want := PlayerSession{ID: 7, AccountID: "1_open-id", PlayerID: 42}

	got, ok := FromData(want)
	if !ok || got != want {
		t.Fatalf("FromData(PlayerSession) = %#v, %t，期望 %#v, true", got, ok, want)
	}

	if _, ok := FromData(models.User{AccountId: want.AccountID, PlayerId: want.PlayerID}); ok {
		t.Fatal("持久化 User 不应被当作 Player Session")
	}
	if _, ok := FromData(nil); ok {
		t.Fatal("nil 不应被当作 Player Session")
	}
}

func TestPlayerIDFromExtractsPlayerID(t *testing.T) {
	want := PlayerSession{PlayerID: 42}
	if got, ok := PlayerIDFrom(want); !ok || got != want.PlayerID {
		t.Fatalf("PlayerIDFrom(PlayerSession) = %d, %t，期望 %d, true", got, ok, want.PlayerID)
	}
	if _, ok := PlayerIDFrom(models.User{PlayerId: want.PlayerID}); ok {
		t.Fatal("持久化 User 不应被解析为 Player ID")
	}
	if _, ok := PlayerIDFrom(nil); ok {
		t.Fatal("nil 不应被解析为 Player ID")
	}
}
