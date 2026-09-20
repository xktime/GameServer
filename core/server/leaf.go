package server

import (
	"context"
	"fmt"
	"gameserver/core/cluster"
	"gameserver/core/conf"
	"gameserver/core/console"
	"gameserver/core/log"
	"gameserver/core/module"
	"time"
)

const shutdownTimeout = 30 * time.Second

type HTTPServer interface {
	Start() error
	Shutdown(context.Context) error
}

func Run(ctx context.Context, httpServer HTTPServer, mods ...module.Module) error {
	// 日志器
	if conf.LogLevel != "" {
		logger, err := log.New(conf.LogLevel, conf.LogPath, conf.LogFlag)
		if err != nil {
			return fmt.Errorf("initialize logger: %w", err)
		}
		log.Export(logger)
		defer logger.Close()
	}

	log.Release("GameServer starting up")

	// 模块
	runtime := module.NewRuntime(mods...)
	runtime.Start()

	// 集群
	cluster.Init()

	// 控制台
	console.Init()
	defer func() {
		console.Destroy()
		cluster.Destroy()
		runtime.Stop()
	}()

	if err := httpServer.Start(); err != nil {
		return fmt.Errorf("start HTTP server: %w", err)
	}

	// 关闭服务。
	<-ctx.Done()
	log.Release("Leaf closing down (%v)", context.Cause(ctx))
	shutdownContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownContext); err != nil {
		return fmt.Errorf("shutdown HTTP server: %w", err)
	}
	return nil
}
