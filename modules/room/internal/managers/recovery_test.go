package managers

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"gameserver/modules/room/matchentry"
	"gameserver/modules/room/participant"
	"slices"
	"strconv"
	"testing"
	"time"
)

func TestRoomRecoveryRetriesCleanupBeforeDisposingQuarantine(t *testing.T) {
	now := time.Unix(100, 0)
	projection := &fakeTeamRoomProjection{}
	var cancelAfterProjection context.CancelFunc
	manager := newTestRoomManager(t, teamRoomProjectionFunc(func(ctx context.Context, desired []participant.DesiredTeamRoom) error {
		if err := projection.Apply(ctx, desired); err != nil {
			return err
		}
		if cancelAfterProjection != nil {
			cancelAfterProjection()
		}
		return nil
	}), &fakePlayerMessenger{}, func() time.Time { return now }, func() string { return "room-1" })
	admission := matchentry.Admission{MatchID: "match-1", Teams: []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}}}
	manager.AcceptMatch(context.Background(), admission)
	previous := manager.rooms["room-1"]
	for range 3 {
		_, err := actor.Command(context.Background(), previous.Ref(), func(actor.Context) (struct{}, error) { panic("quarantine") })
		if !errors.Is(err, actor.ErrHandlerPanic) {
			t.Fatal(err)
		}
	}
	failure := errors.New("projection unavailable")
	projection.err = failure
	if err := manager.Recover(context.Background(), "room-1"); !errors.Is(err, failure) {
		t.Fatalf("failed recovery = %v", err)
	}
	if _, err := manager.roomDefinition.QuarantineTarget(context.Background(), "room-1"); err != nil {
		t.Fatal("room was disposed before Team cleanup succeeded")
	}
	projection.err = nil
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelAfterProjection = cancel
	if err := manager.Recover(ctx, "room-1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled disposal = %v", err)
	}
	cancelAfterProjection = nil
	now = now.Add(MatchTombstoneTTL)
	manager.Maintain()
	if _, retained := manager.closedRooms["room-1"]; !retained {
		t.Fatal("room cleanup record expired before quarantine disposal completed")
	}
	if _, err := manager.roomDefinition.QuarantineTarget(context.Background(), "room-1"); err != nil {
		t.Fatalf("canceled disposal removed the quarantined room: %v", err)
	}
	if err := manager.Recover(context.Background(), "room-1"); err != nil {
		t.Fatal(err)
	}
	if len(manager.playerRooms) != 0 || len(manager.teamRooms) != 0 || len(manager.rooms) != 0 {
		t.Fatal("recovery retained room membership indexes")
	}
	if _, err := manager.roomDefinition.Lookup(context.Background(), "room-1"); !errors.Is(err, actor.ErrActorNotFound) {
		t.Fatalf("quarantined room was not removed: %v", err)
	}
	last := projection.applied[len(projection.applied)-1]
	if len(last) != 1 || last[0].RoomID != "" {
		t.Fatalf("recovery did not clear Team projection: %#v", last)
	}
	manager.Maintain()
	if len(manager.closedRooms) != 0 {
		t.Fatal("completed recovery retained an expired room cleanup record")
	}
}

func TestRoomRecoveryIgnoresUnrelatedProjectionFailures(t *testing.T) {
	for _, scenario := range []string{"first attempt", "cleanup retry", "Team joined another Room"} {
		t.Run(scenario, func(t *testing.T) {
			projection := &fakeTeamRoomProjection{}
			failure := errors.New("projection unavailable")
			failedTeams := make(map[int64]bool)
			nextRoomID := 0
			manager := newTestRoomManager(t, teamRoomProjectionFunc(func(ctx context.Context, desired []participant.DesiredTeamRoom) error {
				if err := projection.Apply(ctx, desired); err != nil {
					return err
				}
				for _, item := range desired {
					if failedTeams[item.TeamID] {
						return failure
					}
				}
				return nil
			}), &fakePlayerMessenger{}, time.Now, func() string {
				nextRoomID++
				return "room-" + strconv.Itoa(nextRoomID)
			})
			for index, teamID := range []int64{7, 8} {
				result := manager.AcceptMatch(context.Background(), matchentry.Admission{
					MatchID: "match-" + strconv.Itoa(index+1),
					Teams:   []matchentry.MatchedTeam{{TeamID: teamID, PlayerIDs: []int64{teamID + 35}}},
				})
				if result.Status != matchentry.Accepted {
					t.Fatalf("AcceptMatch() = %#v", result)
				}
			}
			failedTeams[8] = true
			if !manager.CloseRoom("room-2") {
				t.Fatal("failed to close unrelated Room")
			}
			previous := manager.rooms["room-1"]
			for range 3 {
				_, err := actor.Command(context.Background(), previous.Ref(), func(actor.Context) (struct{}, error) { panic("quarantine") })
				if !errors.Is(err, actor.ErrHandlerPanic) {
					t.Fatal(err)
				}
			}
			target, err := manager.roomDefinition.QuarantineTarget(context.Background(), "room-1")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = manager.roomDefinition.ForceStopQuarantined(context.Background(), target) })
			if scenario != "first attempt" {
				failedTeams[7] = true
				if err := manager.Recover(context.Background(), "room-1"); !errors.Is(err, failure) {
					t.Fatalf("failed recovery = %v", err)
				}
				if _, err := manager.roomDefinition.QuarantineTarget(context.Background(), "room-1"); err != nil {
					t.Fatal("room was disposed before its Team cleanup succeeded")
				}
			}
			wantRoomID := ""
			if scenario == "Team joined another Room" {
				result := manager.AcceptMatch(context.Background(), matchentry.Admission{
					MatchID: "match-3",
					Teams:   []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
				})
				if result.Status != matchentry.Accepted || result.RoomID != "room-3" {
					t.Fatalf("admission after failed cleanup = %#v", result)
				}
				wantRoomID = result.RoomID
			}
			delete(failedTeams, 7)
			attemptCount := len(projection.applied)
			if err := manager.Recover(context.Background(), "room-1"); err != nil {
				t.Fatalf("unrelated Room projection blocked recovery: %v", err)
			}
			attempts := projection.applied[attemptCount:]
			want := []participant.DesiredTeamRoom{{TeamID: 7, RoomID: wantRoomID}}
			if len(attempts) != 1 || !slices.Equal(attempts[0], want) {
				t.Fatalf("recovery should project only the target Room's Team: %#v", attempts)
			}
			if _, err := manager.roomDefinition.Lookup(context.Background(), "room-1"); !errors.Is(err, actor.ErrActorNotFound) {
				t.Fatalf("quarantined room was not removed: %v", err)
			}
			if manager.projectedTeamRoomVersions[8] >= manager.desiredTeamRooms[8].version {
				t.Fatal("recovery marked the unrelated failed projection as applied")
			}
		})
	}
}
