package internal

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"gameserver/core/module"
	"gameserver/modules/room/internal/managers"
	"gameserver/modules/room/matchentry"
	"gameserver/modules/room/participant"
	"testing"
	"time"
)

type failingShutdownProjection struct {
	roomID        string
	clearAttempts int
	failures      int
	err           error
}

func (p *failingShutdownProjection) Apply(ctx context.Context, desired []participant.DesiredTeamRoom) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, item := range desired {
		if item.RoomID == "" {
			p.clearAttempts++
			if p.clearAttempts <= p.failures {
				return p.err
			}
		}
		p.roomID = item.RoomID
	}
	return nil
}

type shutdownProjectionTeam struct {
	actor.BaseActor
	projection  *failingShutdownProjection
	savedRoomID string
}

func (a *shutdownProjectionTeam) OnStop(context.Context) error {
	a.savedRoomID = a.projection.roomID
	return nil
}

type shutdownTestModule struct {
	destroy func()
}

func (*shutdownTestModule) OnInit()                {}
func (*shutdownTestModule) Run(closeSig chan bool) { <-closeSig }
func (m *shutdownTestModule) OnDestroy()           { m.destroy() }

func TestModuleShutdownRetriesProjectionBeforeGamePersistenceAndReportsFailure(t *testing.T) {
	for _, failures := range []int{2, 100} {
		t.Run(map[int]string{2: "temporary", 100: "persistent"}[failures], func(t *testing.T) {
			system := actor.NewActorSystem(time.Second)
			t.Cleanup(func() { _ = system.Stop(context.Background()) })
			roomScope, _ := system.NewScope("room-shutdown")
			gameScope, _ := system.NewScope("game-shutdown")
			projection := &failingShutdownProjection{failures: failures, err: errors.New("projection unavailable")}
			teams, err := actor.Define(gameScope, func(context.Context, int64) (*shutdownProjectionTeam, error) {
				return &shutdownProjectionTeam{projection: projection}, nil
			}, actor.DefinitionOptions{})
			if err != nil {
				t.Fatal(err)
			}
			team, err := teams.GetOrCreate(context.Background(), 7)
			if err != nil {
				t.Fatal(err)
			}
			manager, err := managers.NewRoomManager(roomScope, projection, roomCleanupTestMessenger{}, time.Now, func() string { return "room-1" })
			if err != nil {
				t.Fatal(err)
			}
			result := manager.AcceptMatch(context.Background(), matchentry.Admission{
				MatchID: "match-1", Teams: []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
			})
			if result.Status != matchentry.Accepted {
				t.Fatal(result)
			}
			roomModule := NewModule(roomScope, nil, manager)
			events := []string{}
			roomModule.job = &fakeRoomJob{events: &events}
			gameStopped := false
			// Exercise the same reverse lifecycle order as the service runtime.
			lifecycle := module.NewRuntime(
				&shutdownTestModule{destroy: func() {
					gameStopped = true
					if err := gameScope.Stop(context.Background()); err != nil {
						t.Error(err)
					}
				}},
				&roomShutdownTestModule{Module: roomModule},
			)
			lifecycle.Start()
			shutdownErr := lifecycle.Stop()
			if !gameStopped {
				t.Fatal("room cleanup failure skipped game shutdown")
			}
			if failures == 2 {
				if shutdownErr != nil || team.savedRoomID != "" || projection.clearAttempts < 3 {
					t.Fatalf("shutdown = %v; saved RoomID = %q; cleanup attempts = %d", shutdownErr, team.savedRoomID, projection.clearAttempts)
				}
			} else if !errors.Is(shutdownErr, projection.err) {
				t.Fatalf("shutdown swallowed persistent projection failure: %v", shutdownErr)
			}
		})
	}
}

type roomShutdownTestModule struct{ *Module }

func (*roomShutdownTestModule) OnInit()                {}
func (*roomShutdownTestModule) Run(closeSig chan bool) { <-closeSig }
