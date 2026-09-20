package conf

import (
	"log"
	"time"
)

var (
	// 日志配置
	LogFlag = log.LstdFlags

	// 网关配置
	PendingWriteNum        = 2000
	MaxMsgLen       uint32 = 409600
	HTTPTimeout            = 10 * time.Second
	LenMsgLen              = 4
	LittleEndian           = false

	// 模块骨架配置
	GoLen              = 10000
	TimerDispatcherLen = 10000
	AsynCallLen        = 10000
	ChanRPCLen         = 10000
)
