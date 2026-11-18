package player

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"gameserver/common/models"
	playermodel "gameserver/modules/game/internal/models/player"
	"testing"
)

func TestRecoveryRequiresCleanupAndReloadsPlayerOnNextLogin(t *testing.T) {
	repository := &fakePlayerRepository{loaded: &Player{
		PlayerId: 42, PlayerInfo: &playermodel.PlayerInfo{PlayerName: "persisted"},
	}}
	registry := newPlayerRepositoryTestRegistry(t, repository)
	ctx := context.Background()
	user := models.User{AccountId: "7_recovery", PlayerId: 42}
	prepared, err := registry.PrepareLogin(ctx, user, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	commitTestPlayer(t, prepared, &lifecycleAgent{}, user)
	previous, _ := registry.lookup(ctx, 42)
	t.Cleanup(func() { _ = previous.Ref().ForceStop(ctx) })
	if _, _, err := registry.ModifyName(ctx, 42, "unsaved"); err != nil {
		t.Fatal(err)
	}
	if err := registry.Recover(ctx, 42, func(context.Context) error { t.Fatal("healthy player was cleaned"); return nil }); !errors.Is(err, actor.ErrActorNotQuarantined) {
		t.Fatalf("healthy recovery = %v", err)
	}
	for range 3 {
		_, err := actor.Command(ctx, previous.Ref(), func(actor.Context) (struct{}, error) { panic("quarantine") })
		if !errors.Is(err, actor.ErrHandlerPanic) {
			t.Fatal(err)
		}
	}
	cleanupFailure := errors.New("session cleanup failed")
	if err := registry.Recover(ctx, 42, func(context.Context) error { return cleanupFailure }); !errors.Is(err, cleanupFailure) {
		t.Fatal(err)
	}
	if _, err := registry.Snapshot(ctx, 42); !errors.Is(err, actor.ErrActorQuarantined) {
		t.Fatalf("failed cleanup lost quarantine: %v", err)
	}
	if err := registry.Recover(ctx, 42, func(context.Context) error {
		if _, err := registry.Snapshot(ctx, 42); !errors.Is(err, actor.ErrActorQuarantined) {
			t.Errorf("cleanup must precede disposal: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.lookup(ctx, 42); !errors.Is(err, actor.ErrActorNotFound) || len(repository.saved) != 0 {
		t.Fatalf("recovery activated or saved an unsafe replacement: lookup=%v saves=%v", err, repository.saved)
	}
	prepared, err = registry.PrepareLogin(ctx, user, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	commitTestPlayer(t, prepared, &lifecycleAgent{}, user)
	snapshot, err := registry.Snapshot(ctx, 42)
	if err != nil || snapshot.PlayerInfo.PlayerName != "persisted" || len(repository.loadIDs) != 2 {
		t.Fatalf("next login did not reload durable Player data: %+v, %v, loads=%v", snapshot, err, repository.loadIDs)
	}
}
