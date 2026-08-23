package gate

import (
	"gameserver/common/msg"
	"gameserver/common/msg/message"
	"gameserver/modules/game"
	"gameserver/modules/login"
	"gameserver/modules/match"
	"gameserver/modules/rank"
	"gameserver/modules/room"
)

func InitRouter() {
	// 模块间使用 ChanRPC 通讯，消息路由也不例外
	msg.Processor.SetRouter(&message.C2S_GetUploadUrl{}, game.External.ChanRPC)
	msg.Processor.SetRouter(&message.C2S_GetHistoryRankReward{}, rank.External.ChanRPC)
	msg.Processor.SetRouter(&message.C2S_GetMyHistoryRank{}, rank.External.ChanRPC)
	msg.Processor.SetRouter(&message.C2S_GetChanllengeList{}, rank.External.ChanRPC)
	msg.Processor.SetRouter(&message.C2S_ModifyAvatar{}, game.External.ChanRPC)
	msg.Processor.SetRouter(&message.C2S_ChanllengeByCode{}, rank.External.ChanRPC)
	msg.Processor.SetRouter(&message.C2S_GeneratorChanllengeCode{}, rank.External.ChanRPC)
	msg.Processor.SetRouter(&message.C2S_SeasonInfo{}, rank.External.ChanRPC)
	msg.Processor.SetRouter(&message.C2S_ModifyName{}, game.External.ChanRPC)
	msg.Processor.SetRouter(&message.C2S_CheckName{}, game.External.ChanRPC)
	msg.Processor.SetRouter(&message.C2S_GetPlayerInfo{}, game.External.ChanRPC)
	msg.Processor.SetRouter(&message.C2S_UpdateRankData{}, rank.External.ChanRPC)
	msg.Processor.SetRouter(&message.C2S_GetMyRank{}, rank.External.ChanRPC)
	msg.Processor.SetRouter(&message.C2S_GetRankList{}, rank.External.ChanRPC)
	msg.Processor.SetRouter(&message.C2S_RecordGameOperate{}, room.External.ChanRPC)
	msg.Processor.SetRouter(&message.C2S_CancelMatch{}, match.External.ChanRPC)
	msg.Processor.SetRouter(&message.C2S_StartMatch{}, match.External.ChanRPC)
	msg.Processor.SetRouter(&message.C2S_Login{}, login.External.ChanRPC)
	msg.Processor.SetRouter(&message.C2S_Heart{}, login.External.ChanRPC)
}
