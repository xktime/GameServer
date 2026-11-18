package managers

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	"gameserver/modules/game/internal/managers/player"
	"gameserver/modules/game/internal/managers/team"
	"gameserver/modules/game/internal/provision"
	"sync"
	"testing"
	"time"
)

type blockingNameRepository struct {
	userPlayerRepository
	started chan struct{}
	release chan struct{}
}

func (r *blockingNameRepository) FindByName(context.Context, string) (*player.Player, error) {
	close(r.started)
	<-r.release
	return nil, nil
}

func TestNameLookupDoesNotBlockLoginAndRechecksNewlyOccupiedName(t *testing.T) {
	utils.InitSnowflake(1)
	system := actor.NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	scope, _ := system.NewScope("name-query-login")
	teams, err := team.NewRegistryWithRepository(scope, stubTeamRepository{})
	if err != nil {
		t.Fatal(err)
	}
	repository := &blockingNameRepository{started: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(repository.release) })
	t.Cleanup(release)
	players, err := player.NewRegistryWithRepository(scope, teams, repository)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := newUserManager(context.Background(), scope, players, teams, &fakeUserRepository{}, provision.NewMemoryRepository())
	if err != nil {
		t.Fatal(err)
	}
	checked := make(chan message.Result, 1)
	go func() { checked <- manager.CheckName("available-name") }()
	<-repository.started
	agent := &userManagerAgent{}
	attempt := beginUserLogin(t, manager, agent, 1)
	updated := make(chan error, 1)
	go func() {
		if err := manager.UserLogin(attempt, "different-account", 7, message.LoginType_None); err != nil {
			updated <- err
			return
		}
		active, _ := manager.sessions.Current("7_different-account")
		if result, _ := manager.ModifyName(active.User.PlayerId, "available-name"); result != message.Result_Success {
			t.Errorf("ModifyName = %v", result)
		}
		updated <- nil
	}()
	select {
	case err := <-updated:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		release()
		<-updated
		<-checked
		t.Fatal("name database read blocked an unrelated login/name update")
	}
	release()
	if result := <-checked; result != message.Result_Duplicate {
		t.Fatalf("lookup ignored a name occupied while the database read was pending: %v", result)
	}
}

func TestNameLookupCannotCommitThroughRecoveredUserGeneration(t *testing.T) {
	system := actor.NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	scope, _ := system.NewScope("name-query-recovery")
	teams, err := team.NewRegistryWithRepository(scope, stubTeamRepository{})
	if err != nil {
		t.Fatal(err)
	}
	repository := &blockingNameRepository{started: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(repository.release) })
	t.Cleanup(release)
	players, err := player.NewRegistryWithRepository(scope, teams, repository)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := newUserManager(context.Background(), scope, players, teams, &fakeUserRepository{}, provision.NewMemoryRepository())
	if err != nil {
		t.Fatal(err)
	}
	checked := make(chan message.Result, 1)
	go func() { checked <- manager.CheckName("old-query") }()
	<-repository.started
	previous := manager.currentActor()
	for range 3 {
		_, err := actor.Command(context.Background(), previous.Ref(), func(actor.Context) (struct{}, error) { panic("quarantine") })
		if !errors.Is(err, actor.ErrHandlerPanic) {
			t.Fatal(err)
		}
	}
	if err := manager.Recover(context.Background(), func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	release()
	if result := <-checked; result != message.Result_Fail {
		t.Fatalf("old generation returned a valid name result after recovery: %v", result)
	}
}
