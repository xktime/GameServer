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
	handleMsg(&message.C2S_SeasonInfo{}, handlers.C2S_SeasonInfoHandler)
	handleMsg(&message.C2S_GeneratorChanllengeCode{}, handlers.C2S_GeneratorChanllengeCodeHandler)
	handleMsg(&message.C2S_ChanllengeByCode{}, handlers.C2S_ChanllengeByCodeHandler)
	handleMsg(&message.C2S_GetChanllengeList{}, handlers.C2S_GetChanllengeListHandler)
	handleMsg(&message.C2S_GetMyHistoryRank{}, handlers.C2S_GetMyHistoryRankHandler)
	handleMsg(&message.C2S_GetHistoryRankReward{}, handlers.C2S_GetHistoryRankRewardHandler)
}
