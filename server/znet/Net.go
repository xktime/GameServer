package znet

import (
	"GameServer/server/znet/routers"
	"github.com/aceld/zinx/znet"
)

type Net struct {
}

func Load() {
	//1 创建一个server服务
	s := znet.NewServer()

	// todo: 这里的msgID跟路由绑定，需要整理proto.MessageId
	//2 配置路由
	s.AddRouter(1, &routers.ServerRouter{})

	//3 启动服务
	s.Serve()
}
