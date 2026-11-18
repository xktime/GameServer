package managers

import (
	"context"
	"gameserver/common/base/actor"
	"gameserver/core/log"
	"gameserver/modules/room/participant"
	"time"

	"google.golang.org/protobuf/proto"
)

const MaxRoomLifetime = 30 * time.Minute

type roomActor struct {
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
	ctx context.Context,
	definition *actor.Definition[*roomActor, string],
	roomID string,
	memberIDs []int64,
	realPlayerIDs []int64,
	teamIDs []int64,
	realTeamIDs []int64,
	createdAt time.Time,
) (*roomActor, error) {
	room, err := definition.GetOrCreate(ctx, roomID)
	if err != nil {
		return nil, err
	}
	_, err = actor.Command(ctx, room.Ref(), func(actor.Context) (struct{}, error) {
		room.memberIDs = append([]int64(nil), memberIDs...)
		room.realPlayerIDs = append([]int64(nil), realPlayerIDs...)
		room.teamIDs = append([]int64(nil), teamIDs...)
		room.realTeamIDs = append([]int64(nil), realTeamIDs...)
		room.createdAt = createdAt
		return struct{}{}, nil
	})
	if err != nil {
		return nil, err
	}
	return room, nil
}

func (r *roomActor) send(ctx context.Context, msg proto.Message) {
	_, err := actor.Command(ctx, r.Ref(), func(actor.Context) (struct{}, error) {
		r.messenger.Send(r.realPlayerIDs, msg)
		return struct{}{}, nil
	})
	if err != nil {
		log.Error("发送房间消息失败: %v", err)
	}
}

func (r *roomActor) sendExcept(ctx context.Context, excludedPlayerID int64, msg proto.Message) error {
	_, err := actor.Command(ctx, r.Ref(), func(actor.Context) (struct{}, error) {
		r.messenger.SendExcept(r.realPlayerIDs, excludedPlayerID, msg)
		return struct{}{}, nil
	})
	return err
}
