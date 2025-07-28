package internal

import (
	"gameserver/common/msg/message"
	"gameserver/modules/login/internal/handlers"
	"reflect"

	"google.golang.org/protobuf/proto"
)

func handleMsg(m proto.Message, h interface{}) {
	skeleton.RegisterChanRPC(reflect.TypeOf(m), h)
	// todo handle注册到actor的实现里
}

func init() {
	handleMsg(&message.C2S_Login{}, handlers.HandleLogin)
}
