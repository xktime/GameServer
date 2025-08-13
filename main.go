package main

import (
	"fmt"
	"gameserver/common/config"
	"gameserver/common/db/mongodb"
	"gameserver/common/event_dispatcher"
	"gameserver/common/schedule"
	"gameserver/common/utils"
	"gameserver/conf"
	actor_manager "gameserver/core/actor"
	lconf "gameserver/core/conf"
	"gameserver/core/module"
	"gameserver/core/server"
	"gameserver/gate"
	"gameserver/modules/game"
	"gameserver/modules/login"
	"gameserver/modules/match"
	"gameserver/modules/rank"
	"net/http"
	_ "net/http/pprof"
	"runtime"
)

func main() {
	// 初始化配置
	conf.Instance().Init("./conf")

	// 根据debug配置启用性能分析
	if conf.Server.Debug.Enabled {
		runtime.GOMAXPROCS(1)
		// 启用 mutex 性能分析
		runtime.SetMutexProfileFraction(1)
		// 启用 block 性能分析
		runtime.SetBlockProfileRate(1)
		runtime.SetMutexProfileFraction(1)

		go func() {
			// 启动 http server. 对应 pprof 的一系列 handler 也会挂载在该端口下
			debugAddr := fmt.Sprintf(":%d", conf.Server.Debug.Port)
			if err := http.ListenAndServe(debugAddr, nil); err != nil {
				fmt.Printf("启动debug服务器失败: %v\n", err)
			}
		}()
	}

	Init()

	Run(game.External, login.External, match.External, rank.External)
}

func Run(external ...module.External) {
	//gate放在最后，不用手动注册
	external = append(external, gate.External)
	modules := make([]module.Module, 0, len(external)+1)
	modules = append(modules, event_dispatcher.Module)
	for _, e := range external {
		e.InitExternal()
		modules = append(modules, e.GetModule())
	}
	server.Run(modules...)
}

func Init() {
	// 初始化配置
	lconf.LogLevel = conf.Server.LogLevel
	lconf.LogPath = conf.Server.LogPath
	lconf.LogFlag = conf.LogFlag
	lconf.ConsolePort = conf.Server.ConsolePort
	lconf.ProfilePath = conf.Server.ProfilePath

	config.InitGlobalConfig("./conf/config")

	// 初始化雪花算法
	utils.InitSnowflake(conf.Server.MachineID)

	// 初始化mongodb
	mongodb.Init(conf.Server.MongoDB.Host, conf.Server.MongoDB.Database, conf.Server.MongoDB.MinPoolSize, conf.Server.MongoDB.MaxPoolSize)
	mongodb.CreateIndexes(conf.MongoIndexConf)

	// 初始化actor
	actor_manager.Init(conf.Server.Actor.TimeoutMillisecond)

	// 初始化定时任务
	schedule.Init()
}
