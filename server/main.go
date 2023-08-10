package main

import "github.com/aceld/zinx/znet"

func main() {
	// 创建一个server 句柄
	s := znet.NewServer()
	// 启动server
	s.Serve()
}
