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

// todo user模块和其他模块单独处理
// todo 注册路由时，消息要注册到对应的actor里
// todo 每个消息应该是属于某一个actor,而不是每个消息一个actor
// todo HandleLogin需要是一个actor,需要实现Receive方法。在receive通过消息类型去找相应的handler
func init() {
	// handleMsg(&message.C2S_Login{}, func(args []interface{}) {
	// managers.GetLoginManager().AddToActor(managers.GetLoginManager().DoHandleLogin, args)
	// })
	handleMsg(&message.C2S_Login{}, handlers.DoHandleLogin)
}
