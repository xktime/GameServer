package managers

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"gameserver/modules/room/matchentry"
	"gameserver/modules/room/participant"
	"sync"
	"testing"
	"time"
)

type shutdownTeamProjection struct {
	roomID string
	err    error
}

func (p *shutdownTeamProjection) Apply(ctx context.Context, desired []participant.DesiredTeamRoom) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.err != nil {
		return p.err
	}
	for _, item := range desired {
		p.roomID = item.RoomID
	}
	return nil
}

func TestRoomManagerStopRetriesActorsAfterContextFailure(t *testing.T) {
	for _, failure := range []string{"canceled", "deadline"} {
		t.Run(failure, func(t *testing.T) {
			manager := newTestRoomManager(t, &fakeTeamRoomProjection{}, &fakePlayerMessenger{}, time.Now, func() string { return "room-1" })
			result := manager.AcceptMatch(context.Background(), matchentry.Admission{
				MatchID: "match-1", Teams: []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
			})
			if result.Status != matchentry.Accepted {
				t.Fatalf("room admission = %#v", result)
			}
			ctx, cancel := context.WithCancel(context.Background())
			if failure == "deadline" {
				cancel()
				ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			}
			cancel()
			if err := manager.Stop(ctx); !errors.Is(err, ctx.Err()) {
				t.Fatalf("first Stop = %v; want %v", err, ctx.Err())
			}
			if len(manager.rooms) != 0 || len(manager.playerRooms) != 0 || len(manager.teamRooms) != 0 {
				t.Fatal("stopped manager retained active room membership")
			}
			if err := manager.Stop(context.Background()); err != nil {
				t.Fatalf("retry Stop = %v", err)
			}
			if _, err := manager.roomDefinition.Lookup(context.Background(), result.RoomID); !errors.Is(err, actor.ErrActorNotFound) {
				t.Fatalf("room actor after successful Stop = %v; want ErrActorNotFound", err)
			}
		})
	}
}

func TestRoomManagerStopRetriesProjectionAfterRoomIsRemoved(t *testing.T) {
	for _, failure := range []string{"deadline", "storage"} {
		t.Run(failure, func(t *testing.T) {
			projection := &shutdownTeamProjection{}
			manager := newTestRoomManager(t, projection, &fakePlayerMessenger{}, time.Now, func() string { return "room-1" })
			result := manager.AcceptMatch(context.Background(), matchentry.Admission{
				MatchID: "match-1", Teams: []matchentry.MatchedTeam{{TeamID: 7, PlayerIDs: []int64{42}}},
			})
			if result.Status != matchentry.Accepted || projection.roomID != "room-1" {
				t.Fatalf("room admission = %#v; projection = %q", result, projection.roomID)
			}
			room := manager.rooms["room-1"]
			ctx := context.Background()
			wantErr := errors.New("projection unavailable")
			release := func() {}
			if failure == "deadline" {
				started, blocked := make(chan struct{}), make(chan struct{})
				release = sync.OnceFunc(func() { close(blocked) })
				t.Cleanup(release)
				if err := actor.Tell(context.Background(), room.Ref(), func(actor.Context) error {
					close(started)
					<-blocked
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				<-started
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 20*time.Millisecond)
				defer cancel()
				wantErr = context.DeadlineExceeded
			} else {
				projection.err = wantErr
			}
			if err := manager.Stop(ctx); !errors.Is(err, wantErr) {
				t.Fatalf("first Stop = %v; want %v", err, wantErr)
			}
			if len(manager.rooms) != 0 {
				t.Fatal("closed room remained active")
			}
			projection.err = nil
			release()
			if err := manager.Stop(context.Background()); err != nil {
				t.Fatalf("retry Stop = %v", err)
			}
			if projection.roomID != "" {
				t.Fatalf("successful retry left Team.RoomID = %q", projection.roomID)
			}
			if _, err := manager.roomDefinition.Lookup(context.Background(), result.RoomID); !errors.Is(err, actor.ErrActorNotFound) {
				t.Fatalf("room actor after successful Stop = %v; want ErrActorNotFound", err)
			}
		})
	}
}
