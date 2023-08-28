package main

import (
	"GameServer/client/tests"
	"github.com/aceld/zinx/znet"
)

func main() {
	//创建Client客户端
	client := znet.NewClient("127.0.0.1", 8999)

	//设置链接建立成功后的钩子函数
	client.SetOnConnStart(tests.OnTestLogin)

	//启动客户端
	client.Start()

	//防止进程退出，等待中断信号
	select {}
}
