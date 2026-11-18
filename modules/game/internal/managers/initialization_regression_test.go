package managers

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"gameserver/modules/game/internal/managers/player"
	"gameserver/modules/game/internal/managers/team"
	gameprovision "gameserver/modules/game/internal/provision"
	"sync"
	"testing"
	"time"
)

func TestUserManagerInitializationHonorsCancellationAndDeadline(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		name := "cancellation"
		if deadline {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			system := actor.NewActorSystem(time.Second)
			scope, err := system.NewScope("user-initialization")
			if err != nil {
				t.Fatal(err)
			}
			teams, err := team.NewRegistryWithRepository(scope, stubTeamRepository{})
			if err != nil {
				t.Fatal(err)
			}
			repository := &userPlayerRepository{}
			players, err := player.NewRegistryWithRepository(scope, teams, repository)
			if err != nil {
				t.Fatal(err)
			}
			entered, release := make(chan context.Context, 1), make(chan struct{})
			releaseOnce := sync.OnceFunc(func() { close(release) })
			timeout := 5 * time.Second
			if deadline {
				timeout = 100 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			done := make(chan struct{})
			var initializationError error
			repository.load = func(loadContext context.Context) ([]player.Player, error) {
				entered <- loadContext
				select {
				case <-loadContext.Done():
					return nil, loadContext.Err()
				case <-release:
					return nil, nil
				}
			}
			go func() {
				_, initializationError = newUserManager(ctx, scope, players, teams, &fakeUserRepository{}, gameprovision.NewMemoryRepository())
				close(done)
			}()
			t.Cleanup(func() { cancel(); releaseOnce(); <-done; _ = system.Stop(context.Background()) })
			select {
			case observed := <-entered:
				want, _ := ctx.Deadline()
				if got, ok := observed.Deadline(); !ok || !got.Equal(want) {
					t.Errorf("storage deadline = %v, %t; want %v", got, ok, want)
				}
			case <-time.After(time.Second):
				t.Fatal("initialization did not enter storage")
			}
			if !deadline {
				cancel()
			}
			<-ctx.Done()
			select {
			case <-done:
				if !errors.Is(initializationError, ctx.Err()) {
					t.Fatalf("initialization = %v; want %v", initializationError, ctx.Err())
				}
			case <-time.After(time.Second):
				t.Fatal("initialization remained blocked after its context expired")
			}
		})
	}
}
