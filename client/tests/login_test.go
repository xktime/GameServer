package tests

import (
	"GameServer/server/znet/messages"
	"GameServer/server/znet/messages/proto"
	"GameServer/server/znet/routers/ClientToServer"
	"GameServer/server/znet/routers/ServerToClient"
	"fmt"
	"github.com/aceld/zinx/ziface"
	"github.com/aceld/zinx/znet"
	"testing"
	_ "testing"
	"time"
)

// 创建连接的时候执行
func TestOnTestLogin(t *testing.T) {
	fmt.Println("onClientStart is Called ... ")

	//创建Client客户端
	client := znet.NewClient("127.0.0.1", 8999)

	//设置链接建立成功后的钩子函数
	client.SetOnConnStart(onClientStart)

	// todo 遍历反射设置
	//设置消息读取路由
	login := &ServerToClient.S2CLogin{}
	client.AddRouter(login.GetMessageId(), login)

	//启动客户端
	client.Start()

	//防止进程退出，等待中断信号
	select {}

}

// 创建连接的时候执行
func onClientStart(conn ziface.IConnection) {
	fmt.Println("onClientStart is Called ... ")
	go testLogin(conn)
}

func testLogin(conn ziface.IConnection) {
	for {
		resMessage := &ClientToServer.C2SLogin{
			ProtoMessage: proto.ReqLogin{
				ServerId: 15,
				Account:  "123",
			},
		}

		err := messages.SendMessage(conn, resMessage)
		if err != nil {
			fmt.Println(err)
			break
		}
		time.Sleep(1 * time.Second)
	}
}
