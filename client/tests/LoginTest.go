package tests

import (
	"GameServer/server/znet/messages"
	"encoding/json"
	"fmt"
	"github.com/aceld/zinx/ziface"
	"time"
)

// 创建连接的时候执行
func OnTestLogin(conn ziface.IConnection) {
	fmt.Println("onClientStart is Called ... ")
	go testLogin(conn)
}

// 客户端自定义业务
func testLogin(conn ziface.IConnection) {
	for {
		var login messages.Login
		login.ServerId = 15
		login.Account = 123
		output, _ := json.Marshal(&login)
		err := conn.SendMsg(1, output)
		if err != nil {
			fmt.Println(err)
			break
		}

		time.Sleep(1 * time.Second)
	}
}
