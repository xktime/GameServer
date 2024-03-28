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

	//2 监听客户端发过来的消息
	routers.GetInstance().RegisterC2SRouters(s)

	//3 启动服务 需要用协程来启动 不然会阻塞后面的执行
	go s.Serve()
}
