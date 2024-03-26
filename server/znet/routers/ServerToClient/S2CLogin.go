package ServerToClient

import (
	"GameServer/server/znet/messages"
	"GameServer/server/znet/messages/proto"
	"fmt"
	"github.com/aceld/zinx/ziface"
	"github.com/aceld/zinx/znet"
)

// S2CLogin 登录返回
type S2CLogin struct {
	znet.BaseRouter
	Message proto.ResLogin
}

func (r *S2CLogin) GetMessageId() uint32 {
	return uint32(proto.S2CMessageId_S2C_LOGIN)
}

func (r *S2CLogin) GetProtoMessage() messages.ProtoMessage {
	return &r.Message
}

// S2CLogin Handle 路由处理方法
func (r *S2CLogin) Handle(request ziface.IRequest) {
	fmt.Println("Handle: recv from client : msgId=", request.GetMsgID(), ", data=", string(request.GetData()))
}
