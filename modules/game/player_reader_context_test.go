package game

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	managerplayer "gameserver/modules/game/internal/managers/player"
	"gameserver/modules/game/internal/managers/team"
	"testing"
	"time"
)

type loopingPlayerFinder struct {
	ref actor.ActorRef
}

type failedPlayerFinder struct{ err error }

func (f failedPlayerFinder) GetPlayerSnapshot(context.Context, int64) (managerplayer.Snapshot, bool, error) {
	return managerplayer.Snapshot{}, false, f.err
}

func (f failedPlayerFinder) GetTeamByPlayerID(context.Context, int64) (team.Snapshot, bool, error) {
	return team.Snapshot{}, false, f.err
}

func TestPlayerReadersPreserveLookupErrors(t *testing.T) {
	for _, failure := range []error{context.Canceled, context.DeadlineExceeded, actor.ErrActorQuarantined, errors.New("storage unavailable")} {
		t.Run(failure.Error(), func(t *testing.T) {
			finder := failedPlayerFinder{err: failure}
			match := matchPlayerReader{players: finder, teams: finder}
			if _, found, err := match.FindOnline(context.Background(), 42); found || !errors.Is(err, failure) {
				t.Fatalf("match Player lookup = %t, %v", found, err)
			}
			if _, found, err := match.FindOnlineTeam(context.Background(), 42); found || !errors.Is(err, failure) {
				t.Fatalf("match Team lookup = %t, %v", found, err)
			}
			rank := rankPlayerReader{players: finder}
			if _, found, err := rank.FindOnline(context.Background(), 42); found || !errors.Is(err, failure) {
				t.Fatalf("rank Player lookup = %t, %v", found, err)
			}
		})
	}
}

func (f *loopingPlayerFinder) GetPlayerSnapshot(ctx context.Context, _ int64) (managerplayer.Snapshot, bool, error) {
	_, err := actor.Query(ctx, f.ref, func(actor.Context) (struct{}, error) { return struct{}{}, nil })
	return managerplayer.Snapshot{}, false, err
}

func TestPlayerReadersPreserveSynchronousActorCycleDetection(t *testing.T) {
	for _, name := range []string{"match", "rank"} {
		t.Run(name, func(t *testing.T) {
			system := actor.NewActorSystem(100 * time.Millisecond)
			t.Cleanup(func() { _ = system.Stop(context.Background()) })
			scope, _ := system.NewScope("reader-context")
			definition, _ := actor.Define(scope, func(context.Context, string) (*actor.BaseActor, error) {
				return &actor.BaseActor{}, nil
			}, actor.DefinitionOptions{QueueCapacity: 8})
			instance, _ := definition.GetOrCreate(context.Background(), "source")
			finder := &loopingPlayerFinder{ref: instance.Ref()}
			_, err := actor.Command(context.Background(), instance.Ref(), func(execution actor.Context) (struct{}, error) {
				var err error
				if name == "match" {
					reader := matchPlayerReader{players: finder}
					_, _, err = reader.FindOnline(execution, 42)
				} else {
					reader := rankPlayerReader{players: finder}
					_, _, err = reader.FindOnline(execution, 42)
				}
				return struct{}{}, err
			})
			if !errors.Is(err, actor.ErrCallCycle) {
				t.Fatalf("reader lost the Actor call chain: %v", err)
			}
		})
	}
}
