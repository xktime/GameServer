package msg

import (
	"gameserver/common/msg/message"

	"gameserver/core/network/protobuf"
)

var Processor = protobuf.NewProcessor()

func init() {
	Processor.Register(&message.C2S_Login{})
	Processor.Register(&message.C2S_GetRechargeRecords{})
	Processor.Register(&message.C2S_GetRechargeConfigs{})
	Processor.Register(&message.C2S_RechargeRequest{})
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
