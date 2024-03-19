package znet

import (
	"GameServer/server/znet/messages/proto"
	ServerToClient "GameServer/server/znet/routers/ClientToServer"
	"github.com/aceld/zinx/znet"
)

type Net struct {
}

func Load() {
	//1 创建一个server服务
	s := znet.NewServer()

	//2 配置路由
	s.AddRouter(uint32(proto.C2SMessageId_C2S_LOGIN), &ServerToClient.C2SLogin{})

	//3 启动服务
	s.Serve()
}
