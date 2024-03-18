package routers

import (
	"GameServer/server/znet/messages"
	"fmt"
	"github.com/aceld/zinx/ziface"
	"github.com/aceld/zinx/znet"
)

// ServerRouter MsgId=1的路由
type ServerRouter struct {
	znet.BaseRouter
}

// ServerRouter Handle 路由处理方法
func (r *ServerRouter) Handle(request ziface.IRequest) {
	err := messages.DoAction(request)
	if err != nil {
		fmt.Println("执行异常:", request.GetData())
		return
	}
	fmt.Println("recv from client : msgId=", request.GetMsgID())

}
