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

func InitHandler(users handlers.UserManager) {
	// 向当前模块（game 模块）注册消息处理函数
	handleMsg(&message.C2S_GetPlayerInfo{}, func(args []interface{}) {
		handlers.C2S_GetPlayerInfoHandler(users, args)
	})
	handleMsg(&message.C2S_CheckName{}, func(args []interface{}) {
		handlers.C2S_CheckNameHandler(users, args)
	})
	handleMsg(&message.C2S_ModifyName{}, func(args []interface{}) {
		handlers.C2S_ModifyNameHandler(users, args)
	})
	handleMsg(&message.C2S_GetUploadUrl{}, func(args []interface{}) {
		handlers.C2S_GetUploadUrlHandler(users, args)
	})
	handleMsg(&message.C2S_ModifyAvatar{}, handlers.C2S_ModifyAvatarHandler)
}
