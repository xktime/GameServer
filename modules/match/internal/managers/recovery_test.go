package managers

import (
	"context"
	"errors"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	"gameserver/modules/match/playerread"
	"gameserver/modules/room/matchentry"
	"testing"
	"time"
)

func TestMatchRecoveryRebindsTheExistingManager(t *testing.T) {
	system := actor.NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	scope, _ := system.NewScope("match-recovery")
	players := &fakeMatchPlayerReader{
		players: map[int64]playerread.PlayerSnapshot{42: {TeamID: 7}},
		teams:   map[int64]playerread.TeamSnapshot{42: {TeamID: 7, MemberIDs: []int64{42}}},
	}
	manager, err := NewMatchManager(context.Background(), scope, players, &fakeRoomAcceptor{}, func() string { return "match" }, time.Now)
	if err != nil {
		t.Fatal(err)
	}
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
	teamID, response := manager.HandleMatch(42, &message.C2S_StartMatch{Type: 1})
	if manager.currentActor() == previous || teamID != 7 || response == nil || !response.Result {
		t.Fatalf("recovered match request = %d, %#v", teamID, response)
	}
}

func TestMatchRecoveryPreservesSyntheticIDs(t *testing.T) {
	system := actor.NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	scope, err := system.NewScope("match-recovery-robots")
	if err != nil {
		t.Fatal(err)
	}
	players := &fakeMatchPlayerReader{teams: map[int64]playerread.TeamSnapshot{
		42: {TeamID: 7, MemberIDs: []int64{42}},
		43: {TeamID: 8, MemberIDs: []int64{43}},
	}}
	rooms := &fakeRoomAcceptor{responses: []matchentry.Acceptance{
		{Status: matchentry.Accepted, RoomID: "room-1"},
		{Status: matchentry.Accepted, RoomID: "room-2"},
	}}
	matchID := 0
	manager, err := NewMatchManager(context.Background(), scope, players, rooms, func() string {
		matchID++
		return fmt.Sprintf("match-%d", matchID)
	}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	seenIDs := make(map[int64]bool)
	for index, playerID := range []int64{42, 43} {
		if index > 0 {
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
		}
		if _, response := manager.HandleMatch(playerID, &message.C2S_StartMatch{Type: 1}); response == nil || !response.Result {
			t.Fatalf("start match for %d = %#v", playerID, response)
		}
		manager.OnTimer()
		if _, err := actor.Query(context.Background(), manager.currentActor().Ref(), func(actor.Context) (struct{}, error) { return struct{}{}, nil }); err != nil {
			t.Fatal(err)
		}
		if len(rooms.admissions) != index+1 {
			t.Fatalf("room admissions = %d; want %d", len(rooms.admissions), index+1)
		}
		robots := 0
		for _, team := range rooms.admissions[index].Teams {
			if !team.IsRobot {
				continue
			}
			robots++
			for _, id := range append([]int64{team.TeamID}, team.PlayerIDs...) {
				if id >= 0 || seenIDs[id] {
					t.Fatalf("robot ID %d is invalid or already used by an accepted room", id)
				}
				seenIDs[id] = true
			}
		}
		if robots == 0 {
			t.Fatal("room admission did not exercise robot allocation")
		}
	}
}
