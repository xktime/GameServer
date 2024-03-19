package ServerToClient

import (
	"GameServer/server/db"
	"GameServer/server/znet/messages"
	"GameServer/server/znet/messages/proto"
	"encoding/json"
	"fmt"
	"github.com/aceld/zinx/ziface"
	"github.com/aceld/zinx/znet"
)

// C2SLogin 请求登录
type C2SLogin struct {
	znet.BaseRouter
}

// C2SLogin Handle 路由处理方法
func (r *C2SLogin) Handle(request ziface.IRequest) {
	fmt.Println("receive login message")
	data := request.GetData()
	var message = &proto.ReqLogin{}
	err := json.Unmarshal(data, &message)
	if err != nil {
		return
	}
	user, err := db.FindByAccount(message.GetAccount())
	if err != nil {
		if err.Error() != "mongo: no documents in result" {
			return
		}
		// 如果未注册，先注册再查找
		err = db.RegisterUser(message.GetServerId(), message.GetAccount())
		if err != nil {
			return
		}
		user, _ = db.FindByAccount(message.GetAccount())
	}
	fmt.Println(user)

	err = messages.SendMessage(request.GetConnection(), uint32(proto.C2SMessageId_C2S_LOGIN), &proto.ResLogin{ServerId: message.GetServerId(), Account: message.GetAccount()})
}
