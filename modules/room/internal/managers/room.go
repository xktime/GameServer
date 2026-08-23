package managers

import (
	"gameserver/common/base/actor"
	"gameserver/modules/room/participant"
	"time"

	"google.golang.org/protobuf/proto"
)

const MaxRoomLifetime = 30 * time.Minute

type Room struct {
	actor.BaseActor
	memberIDs     []int64
	realPlayerIDs []int64
	teamIDs       []int64
	realTeamIDs   []int64
	createdAt     time.Time
	maxLifetime   time.Duration
	messenger     participant.PlayerMessenger
}

func createRoom(
	roomID string,
	memberIDs []int64,
	realPlayerIDs []int64,
	teamIDs []int64,
	realTeamIDs []int64,
	createdAt time.Time,
	messenger participant.PlayerMessenger,
) *Room {
	return actor.RegisterActor[*Room](
		actor.Room,
		roomID,
		memberIDs,
		realPlayerIDs,
		teamIDs,
		realTeamIDs,
		createdAt,
		messenger,
	)
}

func (r *Room) Init(args ...any) {
	r.memberIDs = append([]int64(nil), args[0].([]int64)...)
	r.realPlayerIDs = append([]int64(nil), args[1].([]int64)...)
	r.teamIDs = append([]int64(nil), args[2].([]int64)...)
	r.realTeamIDs = append([]int64(nil), args[3].([]int64)...)
	r.createdAt = args[4].(time.Time)
	r.maxLifetime = MaxRoomLifetime
	r.messenger = args[5].(participant.PlayerMessenger)
}

func (r *Room) Stop() {
	r.RemoveActor(r)
}

func (r *Room) send(msg proto.Message) {
	r.SendTask(func() bool {
		r.messenger.Send(r.realPlayerIDs, msg)
		return true
	})
}

func (r *Room) sendExcept(excludedPlayerID int64, msg proto.Message) {
	r.SendTask(func() bool {
		r.messenger.SendExcept(r.realPlayerIDs, excludedPlayerID, msg)
		return true
	})
}
