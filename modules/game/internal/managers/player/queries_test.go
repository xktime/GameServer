package player

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	playermodel "gameserver/modules/game/internal/models/player"
	"testing"
)

func TestStoredQueriesUseRepositoryWithoutActivatingPlayer(t *testing.T) {
	repository := &fakePlayerRepository{loaded: &Player{
		PlayerId: 42, TowerLevel: 9, PlayerInfo: &playermodel.PlayerInfo{PlayerName: "stored"},
	}}
	registry := newPlayerRepositoryTestRegistry(t, repository)
	ctx := context.Background()
	snapshot, err := registry.StoredSnapshot(ctx, 42)
	if err != nil || snapshot.PlayerID != 42 || snapshot.TowerLevel != 9 || snapshot.PlayerInfo.PlayerName != "stored" {
		t.Fatalf("stored snapshot = %+v, %v", snapshot, err)
	}
	snapshot.PlayerInfo.PlayerName = "changed"
	if repository.loaded.PlayerInfo.PlayerName != "stored" {
		t.Fatal("stored snapshot aliases repository data")
	}
	if exists, err := registry.NameExists(ctx, "stored"); err != nil || !exists {
		t.Fatalf("stored name = %t, %v", exists, err)
	}
	if exists, err := registry.NameExists(ctx, "missing"); err != nil || exists {
		t.Fatalf("missing name = %t, %v", exists, err)
	}
	if names, err := registry.StoredNames(ctx); err != nil || len(names) != 1 || names[0] != "stored" {
		t.Fatalf("stored names = %v, %v", names, err)
	}
	if _, err := registry.Snapshot(ctx, 42); !errors.Is(err, actor.ErrActorNotFound) || len(repository.saved) != 0 {
		t.Fatalf("stored queries activated or saved Player: snapshot=%v saves=%v", err, repository.saved)
	}
	for _, info := range []*playermodel.PlayerInfo{nil, {}} {
		repository.loaded.PlayerInfo = info
		if names, err := registry.StoredNames(ctx); err != nil || len(names) != 0 {
			t.Fatalf("stored names for incomplete PlayerInfo = %v, %v", names, err)
		}
	}
	repository.loaded = nil
	if names, err := registry.StoredNames(ctx); err != nil || len(names) != 0 {
		t.Fatalf("stored names for empty repository = %v, %v", names, err)
	}
	if _, err := registry.StoredSnapshot(ctx, 42); !errors.Is(err, actor.ErrActorNotFound) {
		t.Fatalf("missing stored Player = %v", err)
	}
}

func TestStoredQueriesPropagateRepositoryErrors(t *testing.T) {
	for _, failure := range []error{errors.New("storage unavailable"), context.Canceled, context.DeadlineExceeded} {
		t.Run(failure.Error(), func(t *testing.T) {
			repository := &fakePlayerRepository{loadErr: failure}
			registry := newPlayerRepositoryTestRegistry(t, repository)
			if _, err := registry.StoredSnapshot(context.Background(), 42); !errors.Is(err, failure) {
				t.Fatalf("stored snapshot error = %v", err)
			}
			if found, err := registry.NameExists(context.Background(), "stored"); found || !errors.Is(err, failure) {
				t.Fatalf("name lookup = %t, %v", found, err)
			}
			if _, err := registry.StoredNames(context.Background()); !errors.Is(err, failure) {
				t.Fatalf("stored names error = %v", err)
			}
		})
	}
}
