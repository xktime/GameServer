package managers

import (
	"gameserver/common/base/actor"
	"gameserver/modules/game/internal/managers/team"
	"testing"
)

func TestTeamManagerSetRoomProjectionReturnsWhetherTeamExists(t *testing.T) {
	actor.Init(1000)
	t.Cleanup(actor.StopAll)
	teamActor := actor.RegisterActor[*team.Team](actor.Team, int64(7), int64(7), int64(42))
	manager := actor.RegisterActor[*TeamManager](actor.Team, "room-projection-manager")

	applied := manager.SetRoomProjection(7, "room-1")
	missing := manager.SetRoomProjection(8, "room-1")

	if !applied || missing || teamActor.RoomId != "room-1" {
		t.Fatalf("projection results = applied:%t missing:%t RoomId:%q", applied, missing, teamActor.RoomId)
	}
}
