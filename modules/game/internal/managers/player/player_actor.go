
package player

import (
	actor_manager "gameserver/core/actor"
	
	
	
	"google.golang.org/protobuf/proto"
)


// Print 调用Player的Print方法
func Print(PlayerId int64) {
	sendArgs := []interface{}{}
	actor_manager.Send[Player](PlayerId, "Print", sendArgs)
}

// InitTeam 调用Player的InitTeam方法
func InitTeam(PlayerId int64) {
	sendArgs := []interface{}{}
	actor_manager.Send[Player](PlayerId, "InitTeam", sendArgs)
}

// SendToClient 调用Player的SendToClient方法
func SendToClient(PlayerId int64, message proto.Message) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, message)
	
	actor_manager.Send[Player](PlayerId, "SendToClient", sendArgs)
}

