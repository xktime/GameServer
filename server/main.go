package main

import (
	"GameServer/server/common"
	"GameServer/server/db"
	"GameServer/server/znet"
)

func main() {
	// todo: 加载后面要统一处理
	// 加载配置
	common.Load()

	// 加载数据库
	db.Load()

	// 加载znet
	znet.Load()

	select {}
}
