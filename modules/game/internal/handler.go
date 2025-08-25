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
	handleMsg(&message.C2S_CheckName{}, handlers.C2S_CheckNameHandler)
	handleMsg(&message.C2S_ModifyName{}, handlers.C2S_ModifyNameHandler)
	handleMsg(&message.C2S_RechargeRequest{}, handlers.C2S_RechargeRequestHandler)
	handleMsg(&message.C2S_GetRechargeConfigs{}, handlers.C2S_GetRechargeConfigsHandler)
	handleMsg(&message.C2S_GetRechargeRecords{}, handlers.C2S_GetRechargeRecordsHandler)
}
