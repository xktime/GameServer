
package room

import (
	actor_manager "gameserver/core/actor"
	
	
	
	"google.golang.org/protobuf/proto"
)


// SendRoomMessage 调用Room的SendRoomMessage方法
func SendRoomMessage(RoomId int64, msg proto.Message) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, msg)
	
	actor_manager.Send[Room](RoomId, "SendRoomMessage", sendArgs)
}

// PlayerOffline 调用Room的PlayerOffline方法
func PlayerOffline(RoomId int64, playerId int64) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, playerId)
	
	actor_manager.Send[Room](RoomId, "PlayerOffline", sendArgs)
}

