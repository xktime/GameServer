package tests

import (
	"GameServer/server/znet/messages"
	"encoding/json"
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
	client.SetOnConnStart(testLogin)

	//启动客户端
	client.Start()

	//防止进程退出，等待中断信号
	select {}

}

func testLogin(conn ziface.IConnection) {
	for {
		var login messages.ReqLoginMessage
		login.ServerId = 15
		login.Account = 123
		output, _ := json.Marshal(&login)
		err := conn.SendMsg(uint32(login.GetMessageId()), output)
		if err != nil {
			fmt.Println(err)
			break
		}

		time.Sleep(1 * time.Second)
	}
}
