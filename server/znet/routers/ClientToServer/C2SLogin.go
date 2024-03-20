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

	resMessage := messages.NewS2CMessage(proto.S2CMessageId_S2C_LOGIN, &proto.ResLogin{ServerId: message.GetServerId(), Account: message.GetAccount()})
	err = messages.SendMessage(request.GetConnection(), resMessage)
}
