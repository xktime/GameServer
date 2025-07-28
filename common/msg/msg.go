package msg

import (
	"gameserver/common/msg/message"

	"gameserver/core/network/protobuf"
)

var Processor = protobuf.NewProcessor()

func init() {
	// todollw 注册整理，为什么要分开？
	// todollw 生成配置时自动生成配置
	Processor.Register(&message.C2S_Login{})
}
