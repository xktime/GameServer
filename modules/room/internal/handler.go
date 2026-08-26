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

type roomManager interface {
	handlers.RoomManager
	roomOfflineHandler
}

func InitHandler(manager roomManager) {
	handleMsg(&message.C2S_RecordGameOperate{}, func(args []interface{}) {
		handlers.C2S_RecordGameOperateHandler(manager, args)
	})
	registerCloseAgent(manager)
}
