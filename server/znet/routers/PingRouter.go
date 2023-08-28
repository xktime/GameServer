package routers

import (
	"GameServer/server/znet/messages"
	"encoding/json"
	"fmt"
	"github.com/aceld/zinx/ziface"
	"github.com/aceld/zinx/znet"
)

// PingRouter MsgId=1的路由
type PingRouter struct {
	znet.BaseRouter
}

// Ping Handle MsgId=1的路由处理方法
func (r *PingRouter) Handle(request ziface.IRequest) {
	//读取客户端的数据
	var login messages.Login
	err := json.Unmarshal(request.GetData(), &login)
	if err != nil {
		fmt.Println("格式化异常:", request.GetData())
		return
	}
	fmt.Println("recv from client : msgId=", request.GetMsgID(), ", serverId=", login.ServerId, ", Account=", login.Account)
}
