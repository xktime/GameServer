package internal

import (
	"gameserver/common/msg/message"
	"gameserver/modules/login/internal/handlers"
	"reflect"

	"google.golang.org/protobuf/proto"
)

func handleMsg(m proto.Message, h interface{}) {
	skeleton.RegisterChanRPC(reflect.TypeOf(m), h)
}

func InitHandler() {
	handleMsg(&message.C2S_Login{}, handlers.C2S_LoginHandler)
	handleMsg(&message.C2S_Heart{}, handlers.C2S_HeartHandler)
}
