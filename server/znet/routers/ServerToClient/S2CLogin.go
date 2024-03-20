package ServerToClient

import (
	"fmt"
	"github.com/aceld/zinx/ziface"
	"github.com/aceld/zinx/znet"
)

// S2CLogin 登录返回
type S2CLogin struct {
	znet.BaseRouter
}

// S2CLogin Handle 路由处理方法
func (r *S2CLogin) Handle(request ziface.IRequest) {
	fmt.Println("Handle: recv from client : msgId=", request.GetMsgID(), ", data=", string(request.GetData()))
}
