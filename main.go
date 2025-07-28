package main

import (
	"gameserver/common/db/mongodb"
	"gameserver/common/utils"
	"gameserver/conf"
	"gameserver/gate"
	"gameserver/modules/game"
	"gameserver/modules/login"

	actor_manager "gameserver/core/actor"
	lconf "gameserver/core/conf"
	"gameserver/core/server"
)

func main() {

	Init()

	server.Run(
		game.Module,
		gate.Module,
		login.Module,
	)
}

func Init() {
	// 初始化配置
	conf.Instance().Init()
	lconf.LogLevel = conf.Server.LogLevel
	lconf.LogPath = conf.Server.LogPath
	lconf.LogFlag = conf.LogFlag
	lconf.ConsolePort = conf.Server.ConsolePort
	lconf.ProfilePath = conf.Server.ProfilePath

	// 初始化雪花算法
	utils.InitSnowflake(conf.Server.MachineID)

	// 初始化mongodb
	mongodb.Init(conf.Server.MongoDB.Host, conf.Server.MongoDB.Database, conf.Server.MongoDB.MinPoolSize, conf.Server.MongoDB.MaxPoolSize)
	mongodb.CreateIndexes(conf.MongoIndexConf)

	// 初始化actor
	actor_manager.Init(conf.Server.Actor.TimeoutMillisecond)
}
