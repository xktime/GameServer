
package player

import (
	actor_manager "gameserver/core/actor"
	
	
	"google.golang.org/protobuf/proto"
)


// Print 调用Player的Print方法
func Print(PlayerId int64) {
	args := []interface{}{}
	
	actor_manager.Send[Player](PlayerId, "Print", args)
}

// SendToClient 调用Player的SendToClient方法
func SendToClient(PlayerId int64, message proto.Message) {
	args := []interface{}{}
	args = append(args, message)
	
	actor_manager.Send[Player](PlayerId, "SendToClient", args)
}

// PrintJson 调用Player的PrintJson方法
func PrintJson(PlayerId int64, json string) {
	args := []interface{}{}
	args = append(args, json)
	
	actor_manager.Send[Player](PlayerId, "PrintJson", args)
}

