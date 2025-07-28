package gate

import (
	"gameserver/common/msg"
	"gameserver/common/msg/message"
	"gameserver/modules/login"
)

func init() {
	// 模块间使用 ChanRPC 通讯，消息路由也不例外
	msg.Processor.SetRouter(&message.C2S_Login{}, login.ChanRPC)

	// todollw 生成路由表
}
