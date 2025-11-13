package msg

import (
	"gameserver/common/msg/message"

	"gameserver/core/network/protobuf"
)

var Processor = protobuf.NewProcessor()

func init() {
	Processor.Register(&message.C2S_Login{})
	Processor.Register(&message.C2S_GetHistoryRankReward{})
	Processor.Register(&message.C2S_GetMyHistoryRank{})
	Processor.Register(&message.C2S_GetChanllengeList{})
	Processor.Register(&message.C2S_ModifyAvatar{})
	Processor.Register(&message.C2S_GetUploadUrl{})
	Processor.Register(&message.C2S_ChanllengeByCode{})
	Processor.Register(&message.C2S_GeneratorChanllengeCode{})
	Processor.Register(&message.C2S_SeasonInfo{})
	Processor.Register(&message.C2S_ModifyName{})
	Processor.Register(&message.C2S_CheckName{})
	Processor.Register(&message.C2S_GetPlayerInfo{})
	Processor.Register(&message.C2S_UpdateRankData{})
	Processor.Register(&message.C2S_GetMyRank{})
	Processor.Register(&message.C2S_GetRankList{})
	Processor.Register(&message.C2S_RecordGameOperate{})
	Processor.Register(&message.C2S_CancelMatch{})
	Processor.Register(&message.C2S_StartMatch{})
	Processor.Register(&message.C2S_Heart{})
}
