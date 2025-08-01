package internal

import (
	"gameserver/common/msg/message"
	"gameserver/modules/login/internal/managers"
	"reflect"

	"google.golang.org/protobuf/proto"
)

func handleMsg(m proto.Message, h interface{}) {
	skeleton.RegisterChanRPC(reflect.TypeOf(m), h)
}

// todo user模块使用一个额外的层分发代理？
// todo 每个消息应该是属于某一个actor,而不是每个消息一个actor
// todo HandleLogin需要是一个actor,需要实现Receive方法。在receive通过消息类型去找相应的handler
func InitHandler() {
	handleMsg(&message.C2S_Login{}, managers.GetLoginManager().HandleLogin)
}
