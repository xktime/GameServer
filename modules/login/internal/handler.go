package internal

import (
	"gameserver/common/msg/message"
	"gameserver/modules/login/internal/handlers"
	"reflect"

	"google.golang.org/protobuf/proto"
)

func handleMsg(m proto.Message, h interface{}) {
	skeleton.RegisterAsyncChanRPC(reflect.TypeOf(m), h)
}

func InitHandler(login handlers.LoginManager, connections handlers.ConnectManager) {
	handleMsg(&message.C2S_Login{}, func(args []interface{}) {
		handlers.C2S_LoginHandler(login, connections, args)
	})
	handleMsg(&message.C2S_Heart{}, func(args []interface{}) {
		handlers.C2S_HeartHandler(connections, args)
	})
	registerAgentRPC(connections)
}
