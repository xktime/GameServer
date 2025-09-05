package room

import (
	"gameserver/common/base/actor"

	"google.golang.org/protobuf/proto"
)

// GetInterval 调用Room的GetInterval方法
func GetInterval(RoomId int64) int {
	if room, ok := actor.GetActor[Room](actor.Room, RoomId); ok {
		return room.GetInterval()
	}
	return 0
}

// OnTimer 调用Room的OnTimer方法
func OnTimer(RoomId int64) {
	if room, ok := actor.GetActor[Room](actor.Room, RoomId); ok {
		room.OnTimer()
	}
}

// CheckExpiration 调用Room的CheckExpiration方法
func CheckExpiration(RoomId int64) {
	if room, ok := actor.GetActor[Room](actor.Room, RoomId); ok {
		room.CheckExpiration()
	}
}

// Stop 调用Room的StopRoom方法
func Stop(RoomId int64) {
	if room, ok := actor.GetActor[Room](actor.Room, RoomId); ok {
		room.StopRoom()
	}
}

// IsExpired 调用Room的IsExpired方法
func IsExpired(RoomId int64) bool {
	if room, ok := actor.GetActor[Room](actor.Room, RoomId); ok {
		return room.IsExpired()
	}
	return true
}

// SendRoomMessage 调用Room的SendRoomMessage方法
func SendRoomMessage(RoomId int64, msg proto.Message) {
	if room, ok := actor.GetActor[Room](actor.Room, RoomId); ok {
		room.SendRoomMessage(msg)
	}
}

// SendRoomMessageExceptSelf 调用Room的SendRoomMessageExceptSelf方法
func SendRoomMessageExceptSelf(RoomId int64, msg proto.Message, selfId int64) {
	if room, ok := actor.GetActor[Room](actor.Room, RoomId); ok {
		room.SendRoomMessageExceptSelf(msg, selfId)
	}
}

// PlayerOffline 调用Room的PlayerOffline方法
func PlayerOffline(RoomId int64, playerId int64) {
	if room, ok := actor.GetActor[Room](actor.Room, RoomId); ok {
		room.PlayerOffline(playerId)
	}
}
