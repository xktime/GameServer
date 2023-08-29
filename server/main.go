package main

import (
	"GameServer/server/common"
	"GameServer/server/znet"
)

func main() {
	// 加载配置
	new(common.Config).Load()
	// 加载znet
	new(znet.Net).Load()
}
