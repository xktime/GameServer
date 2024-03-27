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

	//2 配置路由
	routers.GetInstance().RegisterC2SRouters(s)

	//3 启动服务
	s.Serve()
}
