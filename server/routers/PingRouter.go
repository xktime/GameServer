package routers

import (
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
	fmt.Println("recv from client : msgId=", request.GetMsgID(), ", data=", string(request.GetData()))
}
