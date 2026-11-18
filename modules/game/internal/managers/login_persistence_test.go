package managers

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	"gameserver/modules/game/internal/managers/player"
	"gameserver/modules/game/internal/managers/team"
	gameprovision "gameserver/modules/game/internal/provision"
	"testing"
	"time"
)

type offlineSaveUserRepository struct{ fakeUserRepository }

func (r *offlineSaveUserRepository) FindByLogin(context.Context, string, int32) (*models.User, error) {
	if len(r.saved) == 0 {
		return nil, nil
	}
	user := r.saved[len(r.saved)-1]
	return &user, nil
}

type offlineSavePlayerRepository struct {
	userPlayerRepository
	saveErr      error
	saveAttempts int
}

func (r *offlineSavePlayerRepository) Save(_ context.Context, current *player.Player) error {
	r.saveAttempts++
	if r.saveErr != nil {
		return r.saveErr
	}
	stored := *current
	if current.PlayerInfo != nil {
		info := *current.PlayerInfo
		stored.PlayerInfo = &info
	}
	r.stored = &stored
	return nil
}

func TestLoginRetriesFailedOfflineSaveWithoutLosingPlayerState(t *testing.T) {
	utils.InitSnowflake(1)
	ctx := context.Background()
	system := actor.NewActorSystem(time.Second)
	repository := &offlineSavePlayerRepository{}
	t.Cleanup(func() {
		repository.saveErr = nil
		if err := system.Stop(ctx); err != nil {
			t.Errorf("Stop ActorSystem: %v", err)
		}
	})
	scope, err := system.NewScope("login-offline-save-retry")
	if err != nil {
		t.Fatal(err)
	}
	teams, err := team.NewRegistryWithRepository(scope, stubTeamRepository{})
	if err != nil {
		t.Fatal(err)
	}
	players, err := player.NewRegistryWithRepository(scope, teams, repository)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := newUserManager(ctx, scope, players, teams, &offlineSaveUserRepository{}, gameprovision.NewMemoryRepository())
	if err != nil {
		t.Fatal(err)
	}
	first := &userManagerAgent{}
	if response := testUserLogin(t, manager, first, "save-retry", 7, message.LoginType_None); response.LoginResult != 1 {
		t.Fatalf("first login = %#v", response)
	}
	active, ok := manager.LoginState().Current("7_save-retry")
	if !ok {
		t.Fatal("first login did not establish a session")
	}
	playerID := active.User.PlayerId
	before, err := players.Snapshot(ctx, playerID)
	if err != nil {
		t.Fatal(err)
	}
	// A teammate keeps the Team active while this Player reconnects.
	if err := teams.Join(ctx, before.TeamID, 43); err != nil {
		t.Fatal(err)
	}
	if result, _, err := players.ModifyName(ctx, playerID, "unsaved-name"); err != nil || result != message.Result_Success {
		t.Fatalf("modify name = %v, %v", result, err)
	}
	saveFailure := errors.New("temporary save failure")
	repository.saveErr = saveFailure
	manager.ClosePlayerSession(active.Snapshot)
	settleUser(t, manager)
	if _, online := manager.LoginState().Current("7_save-retry"); online || repository.saveAttempts != 1 || repository.stored != nil {
		t.Fatal("failed offline save did not retain unsaved Player state outside a session")
	}
	failed := &userManagerAgent{}
	attempt := beginUserLogin(t, manager, failed, 2)
	if err := manager.UserLogin(attempt, "save-retry", 7, message.LoginType_None); !errors.Is(err, saveFailure) {
		t.Fatalf("login while save still fails = %v; want %v", err, saveFailure)
	}
	attempt.Fail()
	if _, online := manager.LoginState().Current("7_save-retry"); online || failed.UserData() != nil || repository.stored != nil {
		t.Fatal("failed save retry established a session or discarded unsaved Player state")
	}
	repository.saveErr = nil
	second := &userManagerAgent{}
	if response := testUserLogin(t, manager, second, "save-retry", 7, message.LoginType_None); response.LoginResult != 1 {
		t.Fatalf("login after storage recovery = %#v", response)
	}
	snapshot, err := players.Snapshot(ctx, playerID)
	if err != nil || snapshot.TeamID != before.TeamID || snapshot.PlayerInfo == nil || snapshot.PlayerInfo.PlayerName != "unsaved-name" {
		t.Fatalf("relogin lost unsaved Player data: %#v, %v", snapshot, err)
	}
	if repository.saveAttempts != 3 || repository.stored == nil || repository.stored.PlayerInfo.PlayerName != "unsaved-name" {
		t.Fatalf("offline save was not retried before reloading: attempts=%d stored=%#v", repository.saveAttempts, repository.stored)
	}
	current, ok := manager.LoginState().Current("7_save-retry")
	if !ok || current.Snapshot == active.Snapshot || second.UserData() != current.Snapshot {
		t.Fatal("relogin did not establish a new Player Session")
	}
}
