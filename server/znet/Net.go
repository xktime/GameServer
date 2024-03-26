package znet

import (
	ServerToClient "GameServer/server/znet/routers/ClientToServer"
	"github.com/aceld/zinx/znet"
)

type Net struct {
}

func Load() {
	//1 创建一个server服务
	s := znet.NewServer()

	// todo 遍历反射设置
	//2 配置路由
	login := &ServerToClient.C2SLogin{}
	s.AddRouter(login.GetMessageId(), login)

	//3 启动服务
	s.Serve()
}
