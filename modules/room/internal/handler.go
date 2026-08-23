package internal

import (
	"gameserver/common/msg/message"
	"gameserver/modules/room/internal/handlers"
	"reflect"

	"google.golang.org/protobuf/proto"
)

func handleMsg(msg proto.Message, handler interface{}) {
	skeleton.RegisterChanRPC(reflect.TypeOf(msg), handler)
}

func InitHandler() {
	handleMsg(&message.C2S_RecordGameOperate{}, handlers.C2S_RecordGameOperateHandler)
}
