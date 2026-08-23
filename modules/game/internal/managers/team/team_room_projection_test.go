package team

import (
	"gameserver/common/base/actor"
	"testing"
)

func TestSetRoomProjectionSynchronouslyAppliesLatestDesiredValue(t *testing.T) {
	actor.Init(1000)
	t.Cleanup(actor.StopAll)
	team := actor.RegisterActor[*Team](actor.Team, int64(7), int64(7), int64(42))

	first := team.SetRoomProjection("room-1")
	repeated := team.SetRoomProjection("room-1")
	cleared := team.SetRoomProjection("")

	if !first || !repeated || !cleared || team.RoomId != "" {
		t.Fatalf("projection results = first:%t repeated:%t cleared:%t RoomId:%q", first, repeated, cleared, team.RoomId)
	}
}
