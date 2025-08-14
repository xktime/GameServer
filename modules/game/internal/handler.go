package internal

import (
	"reflect"

	"gameserver/common/msg/message"
	"gameserver/modules/game/internal/handlers"

	"google.golang.org/protobuf/proto"
)

func handleMsg(m proto.Message, h interface{}) {
	skeleton.RegisterChanRPC(reflect.TypeOf(m), h)
}

func InitHandler() {
	// 向当前模块（game 模块）注册消息处理函数
	handleMsg(&message.C2S_GetPlayerInfo{}, handlers.C2S_GetPlayerInfoHandler)
}
