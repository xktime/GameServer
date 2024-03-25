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

	// 加载znet 需要用协程来启动 不然会阻塞后面的执行
	go znet.Load()

	select {}
}
