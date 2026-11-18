package managers

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	"gameserver/modules/game/internal/provision"
	"testing"
)

func TestPlayerFindersDistinguishOfflineFromCanceledAndQuarantined(t *testing.T) {
	users := newUserManagerRepositoryTestHarness(t, &fakeUserRepository{}, provision.NewMemoryRepository())
	teams, err := NewTeamManager(users.currentActor().players, users.currentActor().teams)
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := users.GetPlayerSnapshot(context.Background(), 42); found || err != nil {
		t.Fatalf("offline Player = %t, %v", found, err)
	}
	if _, found, err := teams.GetTeamByPlayerID(context.Background(), 42); found || err != nil {
		t.Fatalf("offline Team = %t, %v", found, err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, found, err := users.GetPlayerSnapshot(canceled, 42); found || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Player = %t, %v", found, err)
	}
	if _, found, err := teams.GetTeamByPlayerID(canceled, 42); found || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Team = %t, %v", found, err)
	}
	agent := &userManagerAgent{}
	if response := testUserLogin(t, users, agent, "quarantined-reader", 7, message.LoginType_None); response.LoginResult != 1 {
		t.Fatal(response)
	}
	active, _ := users.LoginState().Current("7_quarantined-reader")
	playerID := active.User.PlayerId
	t.Cleanup(func() {
		if err := users.RecoverPlayer(context.Background(), playerID, func(context.Context) error { return nil }); err != nil {
			t.Error(err)
		}
	})
	agent.beforeWrite = func() { panic("transport panic") }
	for range 3 {
		if err := users.currentActor().players.SendToClient(context.Background(), playerID, &message.S2C_Heart{}); err != nil && !errors.Is(err, actor.ErrActorQuarantined) {
			t.Fatal(err)
		}
	}
	if _, found, err := users.GetPlayerSnapshot(context.Background(), playerID); found || !errors.Is(err, actor.ErrActorQuarantined) {
		t.Fatalf("quarantined Player = %t, %v", found, err)
	}
	if _, found, err := teams.GetTeamByPlayerID(context.Background(), playerID); found || !errors.Is(err, actor.ErrActorQuarantined) {
		t.Fatalf("quarantined Team lookup = %t, %v", found, err)
	}
}
