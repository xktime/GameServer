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

func InitHandler() {
	handleMsg(&message.C2S_StartMatch{}, handlers.C2S_StartMatchHandler)
	handleMsg(&message.C2S_CancelMatch{}, handlers.C2S_CancelMatchHandler)
}
