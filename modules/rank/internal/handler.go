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

func InitHandler(manager handlers.RankManager) {
	handleMsg(&message.C2S_GetRankList{}, func(args []interface{}) { handlers.C2S_GetRankListHandler(manager, args) })
	handleMsg(&message.C2S_GetMyRank{}, func(args []interface{}) { handlers.C2S_GetMyRankHandler(manager, args) })
	handleMsg(&message.C2S_UpdateRankData{}, func(args []interface{}) { handlers.C2S_UpdateRankDataHandler(manager, args) })
	handleMsg(&message.C2S_SeasonInfo{}, func(args []interface{}) { handlers.C2S_SeasonInfoHandler(manager, args) })
	handleMsg(&message.C2S_GeneratorChanllengeCode{}, func(args []interface{}) { handlers.C2S_GeneratorChanllengeCodeHandler(manager, args) })
	handleMsg(&message.C2S_ChanllengeByCode{}, func(args []interface{}) { handlers.C2S_ChanllengeByCodeHandler(manager, args) })
	handleMsg(&message.C2S_GetChanllengeList{}, func(args []interface{}) { handlers.C2S_GetChanllengeListHandler(manager, args) })
	handleMsg(&message.C2S_GetMyHistoryRank{}, func(args []interface{}) { handlers.C2S_GetMyHistoryRankHandler(manager, args) })
	handleMsg(&message.C2S_GetHistoryRankReward{}, func(args []interface{}) { handlers.C2S_GetHistoryRankRewardHandler(manager, args) })
}
