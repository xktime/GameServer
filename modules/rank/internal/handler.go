package internal

import (
	"gameserver/common/msg/message"
	"gameserver/modules/rank/internal/handlers"
	"reflect"

	"google.golang.org/protobuf/proto"
)

func handleMsg(m proto.Message, h interface{}) {
	skeleton.RegisterChanRPC(reflect.TypeOf(m), h)
}

func InitHandler() {
	handleMsg(&message.C2S_GetRankList{}, handlers.C2S_GetRankListHandler)
	handleMsg(&message.C2S_GetMyRank{}, handlers.C2S_GetMyRankHandler)
	handleMsg(&message.C2S_UpdateRankData{}, handlers.C2S_UpdateRankDataHandler)
}
