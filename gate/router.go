package gate

import (
	"gameserver/common/msg"
	"gameserver/common/msg/message"
	"gameserver/modules/login"
	"gameserver/modules/match"
)

func InitRouter() {
	// 模块间使用 ChanRPC 通讯，消息路由也不例外
	msg.Processor.SetRouter(&message.C2S_Login{}, login.External.ChanRPC)
	msg.Processor.SetRouter(&message.C2S_RecordGameOperate{}, match.External.ChanRPC)
	msg.Processor.SetRouter(&message.C2S_CancelMatch{}, match.External.ChanRPC)
	msg.Processor.SetRouter(&message.C2S_StartMatch{}, match.External.ChanRPC)
	msg.Processor.SetRouter(&message.C2S_Heart{}, login.External.ChanRPC)
	msg.Processor.SetRouter(&message.C2S_Reconnect{}, login.External.ChanRPC)
}
