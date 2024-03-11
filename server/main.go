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
	// todo: znet Load会阻塞 停止之后才会执行之后的逻辑
	// 加载znet
	znet.Load()
}
