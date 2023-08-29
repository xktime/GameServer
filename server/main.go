package main

import (
	"GameServer/server/common"
	"GameServer/server/db"
	"GameServer/server/znet"
)

func main() {
	// todo 加载方式要修改 单例
	// 加载配置
	new(common.Config).Load()
	// 加载数据库
	new(db.DBManager).Load()
	// 加载znet
	new(znet.Net).Load()
}
