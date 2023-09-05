package routers

import (
	"GameServer/server/znet/messages"
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
	err := messages.DoAction(messages.MessageId(request.GetMsgID()), request.GetData())
	if err != nil {
		fmt.Println("执行异常:", request.GetData())
		return
	}
	fmt.Println("recv from client : msgId=", request.GetMsgID())
}
