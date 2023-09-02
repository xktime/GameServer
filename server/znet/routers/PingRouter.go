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
	// todo 要根据msgId转发到对应的模块处理
	//读取客户端的数据
	message := messages.GetMessage(request.GetMsgID())
	err := json.Unmarshal(request.GetData(), &message)
	if err != nil {
		fmt.Println("格式化异常:", request.GetData())
		return
	}
	fmt.Println("recv from client : msgId=", request.GetMsgID(), ", serverId=", message)
}
