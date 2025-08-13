
package room

import (
	actor_manager "gameserver/core/actor"
	
	
	
	
	"google.golang.org/protobuf/proto"
)


// CheckExpiration 调用Room的CheckExpiration方法
func CheckExpiration(RoomId int64) (bool) {
	sendArgs := []interface{}{}
	future := actor_manager.RequestFuture[Room](RoomId, "CheckExpiration", sendArgs)
	result, _ := future.Result()
	return result.(bool)
}

// Stop 调用Room的Stop方法
func Stop(RoomId int64) {
	sendArgs := []interface{}{}
	actor_manager.Send[Room](RoomId, "Stop", sendArgs)
}

// IsExpired 调用Room的IsExpired方法
func IsExpired(RoomId int64) (bool) {
	sendArgs := []interface{}{}
	future := actor_manager.RequestFuture[Room](RoomId, "IsExpired", sendArgs)
	result, _ := future.Result()
	return result.(bool)
}

// SendRoomMessage 调用Room的SendRoomMessage方法
func SendRoomMessage(RoomId int64, msg proto.Message) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, msg)
	
	actor_manager.Send[Room](RoomId, "SendRoomMessage", sendArgs)
}

// SendRoomMessageExceptSelf 调用Room的SendRoomMessageExceptSelf方法
func SendRoomMessageExceptSelf(RoomId int64, msg proto.Message, selfId int64) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, msg)
	sendArgs = append(sendArgs, selfId)
	
	actor_manager.Send[Room](RoomId, "SendRoomMessageExceptSelf", sendArgs)
}

// PlayerOffline 调用Room的PlayerOffline方法
func PlayerOffline(RoomId int64, playerId int64) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, playerId)
	
	actor_manager.Send[Room](RoomId, "PlayerOffline", sendArgs)
}

