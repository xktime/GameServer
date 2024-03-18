package routers

import (
	"fmt"
	"github.com/aceld/zinx/ziface"
	"github.com/aceld/zinx/znet"
)

// ClientRouter MsgId=1的路由
type ClientRouter struct {
	znet.BaseRouter
}

// ClientRouter Handle 路由处理方法
func (r *ClientRouter) Handle(request ziface.IRequest) {
	fmt.Println("Handle: recv from client : msgId=", request.GetMsgID(), ", data=", string(request.GetData()))
}
