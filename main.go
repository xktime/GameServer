package main

import (
	"context"
	"errors"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/bucket"
	"gameserver/common/config"
	"gameserver/common/db/mongodb"
	"gameserver/common/event_dispatcher"
	"gameserver/common/schedule"
	"gameserver/common/utils"
	"gameserver/conf"
	lconf "gameserver/core/conf"
	"gameserver/core/module"
	"gameserver/core/server"
	"gameserver/gate"
	gamehttp "gameserver/http"
	"gameserver/modules/game"
	"gameserver/modules/login"
	"gameserver/modules/match"
	"gameserver/modules/rank"
	"gameserver/modules/room"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"time"
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

	if err := Init(); err != nil {
		closeContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		panic(errors.Join(err, mongodb.Close(closeContext)))
	}

	system := actor.NewActorSystem(time.Duration(conf.Server.Actor.TimeoutMillisecond) * time.Millisecond)
	runErr := Run(system, game.External, login.External, room.External, match.External, rank.External)
	closeContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := errors.Join(runErr, mongodb.Close(closeContext)); err != nil {
		panic(err)
	}
}

func Run(system *actor.ActorSystem, external ...module.External) error {
	if system == nil {
		return fmt.Errorf("main: ActorSystem is nil")
	}
	//gate放在最后，不用手动注册
	external = append(external, gate.External)
	modules := make([]module.Module, 0, len(external)+1)
	modules = append(modules, event_dispatcher.Module)
	for _, e := range external {
		if err := e.InitExternal(system); err != nil {
			stopContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			return errors.Join(fmt.Errorf("initialize external %T: %w", e, err), system.Stop(stopContext))
		}
		modules = append(modules, e.GetModule())
	}
	server.Run(modules...)
	stopContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return system.Stop(stopContext)
}

func Init() error {
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
	if err := mongodb.Init(conf.Server.MongoDB.Host, conf.Server.MongoDB.Database, conf.Server.MongoDB.MinPoolSize, conf.Server.MongoDB.MaxPoolSize); err != nil {
		return err
	}
	if err := mongodb.CreateIndexes(conf.MongoIndexConf); err != nil {
		return err
	}

	// 初始化OSS
	if err := bucket.GetOSSClient().Init(bucket.OSSConfig{
		AccessKeyID:     conf.Server.Bucket.AccessKeyID,
		AccessKeySecret: conf.Server.Bucket.AccessKeySecret,
		Endpoint:        conf.Server.Bucket.Endpoint,
		BucketName:      conf.Server.Bucket.BucketName,
	}); err != nil {
		return err
	}

	// 初始化定时任务
	schedule.Init()

	// 初始化http
	gamehttp.Start()
	return nil
}
