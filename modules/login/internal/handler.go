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

// todo 根据type分发？
// todo  handleMsg(m proto.Message) {
//	skeleton.RegisterChanRPC(reflect.TypeOf(m), getDispacher(m.getType()))
//}
func InitHandler() {
	handleMsg(&message.C2S_Login{}, handlers.C2S_LoginHandler)
}
// todo 根据消息去给后面拼接一个handler生成到msg.message.handlers?比如 C2S_Login默认指向C2S_LoginHandler
