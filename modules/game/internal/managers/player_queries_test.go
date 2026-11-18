package managers

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	"gameserver/modules/game/internal/managers/player"
	"gameserver/modules/game/internal/managers/team"
	playermodel "gameserver/modules/game/internal/models/player"
	"gameserver/modules/game/internal/provision"
	"testing"
	"time"
)

type userPlayerRepository struct {
	stored    *player.Player
	err       error
	load      func(context.Context) ([]player.Player, error)
	loadCalls int
	nameCalls []string
}

func (r *userPlayerRepository) FindByID(context.Context, int64) (*player.Player, error) {
	return r.stored, r.err
}

func (r *userPlayerRepository) FindByName(_ context.Context, name string) (*player.Player, error) {
	r.nameCalls = append(r.nameCalls, name)
	return r.stored, r.err
}

func (r *userPlayerRepository) FindAll(ctx context.Context) ([]player.Player, error) {
	r.loadCalls++
	if r.load != nil {
		return r.load(ctx)
	}
	if r.stored == nil {
		return nil, r.err
	}
	return []player.Player{*r.stored}, r.err
}

func (*userPlayerRepository) Save(context.Context, *player.Player) error { return nil }

func TestUserManagerQueriesInjectedPlayerRepository(t *testing.T) {
	system := actor.NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	scope, _ := system.NewScope("user-player-queries")
	teams, _ := team.NewRegistryWithRepository(scope, stubTeamRepository{})
	repository := &userPlayerRepository{stored: &player.Player{
		PlayerId: 42, PlayerInfo: &playermodel.PlayerInfo{PlayerId: 42, PlayerName: "stored"},
	}}
	players, err := player.NewRegistryWithRepository(scope, teams, repository)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := newUserManager(context.Background(), scope, players, teams, &fakeUserRepository{}, provision.NewMemoryRepository())
	if err != nil {
		t.Fatal(err)
	}
	if repository.loadCalls != 1 || manager.CheckName("stored") != message.Result_Duplicate {
		t.Fatalf("name preload bypassed Player repository: loads=%d", repository.loadCalls)
	}
	if info, found := manager.GetPlayerInfo(42); !found || info.PlayerName != "stored" {
		t.Fatalf("offline info bypassed Player repository: %v, %t", info, found)
	}
	if _, err := players.Snapshot(context.Background(), 42); !errors.Is(err, actor.ErrActorNotFound) {
		t.Fatalf("offline query activated Player: %v", err)
	}
	// Names not cached during preload must consult the same injected repository.
	if result := manager.CheckName("new-name"); result != message.Result_Duplicate || len(repository.nameCalls) != 1 || repository.nameCalls[0] != "new-name" {
		t.Fatalf("name query = %v; repository calls = %v", result, repository.nameCalls)
	}
	repository.err = errors.New("storage unavailable")
	if _, found := manager.GetPlayerInfo(42); found {
		t.Fatal("failed offline read returned a Player")
	}
	if result := manager.CheckName("failed-name"); result != message.Result_Fail {
		t.Fatalf("failed name query = %v", result)
	}
}

func TestUserManagerDoesNotCacheMissingNames(t *testing.T) {
	system := actor.NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	scope, _ := system.NewScope("user-name-cache")
	teams, _ := team.NewRegistry(scope)
	repository := &userPlayerRepository{}
	players, err := player.NewRegistryWithRepository(scope, teams, repository)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := newUserManager(context.Background(), scope, players, teams, &fakeUserRepository{}, provision.NewMemoryRepository())
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if result := manager.CheckName("available-name"); result != message.Result_Success {
			t.Fatalf("missing name = %v", result)
		}
	}
	if len(repository.nameCalls) != 2 {
		t.Fatalf("missing name was cached: repository calls = %v", repository.nameCalls)
	}
	repository.stored = &player.Player{PlayerId: 42, PlayerInfo: &playermodel.PlayerInfo{PlayerName: "available-name"}}
	if result := manager.CheckName("available-name"); result != message.Result_Duplicate {
		t.Fatalf("newly occupied name = %v", result)
	}
	repository.err = errors.New("storage unavailable")
	if result := manager.CheckName("available-name"); result != message.Result_Duplicate {
		t.Fatalf("occupied name was not cached: %v", result)
	}
}
