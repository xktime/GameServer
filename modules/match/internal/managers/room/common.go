package room

import (
	"gameserver/common/base/actor"

	"google.golang.org/protobuf/proto"
)

// SendRoomMessage 调用Room的SendRoomMessage方法
func SendRoomMessage(RoomId string, msg proto.Message) {
	if room, ok := actor.GetActor[Room](actor.Room, RoomId); ok {
		room.SendRoomMessage(msg)
	}
}

// SendRoomMessageExceptSelf 调用Room的SendRoomMessageExceptSelf方法
func SendRoomMessageExceptSelf(RoomId string, msg proto.Message, selfId int64) {
	if room, ok := actor.GetActor[Room](actor.Room, RoomId); ok {
		room.SendRoomMessageExceptSelf(msg, selfId)
	}
}

// PlayerOffline 调用Room的PlayerOffline方法
func PlayerOffline(RoomId string, playerId int64) {
	if room, ok := actor.GetActor[Room](actor.Room, RoomId); ok {
		room.PlayerOffline(playerId)
	}
}
