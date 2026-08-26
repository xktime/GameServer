package internal

import (
	"gameserver/common/msg/message"
	"gameserver/modules/match/internal/handlers"
	"reflect"

	"google.golang.org/protobuf/proto"
)

func handleMsg(m proto.Message, h interface{}) {
	skeleton.RegisterChanRPC(reflect.TypeOf(m), h)
}

func InitHandler(manager handlers.MatchManager, teams handlers.TeamMessenger) {
	handleMsg(&message.C2S_StartMatch{}, func(args []interface{}) {
		handlers.C2S_StartMatchHandler(manager, teams, args)
	})
	handleMsg(&message.C2S_CancelMatch{}, func(args []interface{}) {
		handlers.C2S_CancelMatchHandler(manager, teams, args)
	})
}
