package messages

import (
	"GameServer/server/db"
	"GameServer/server/znet/messages/proto"
	"fmt"
	"github.com/aceld/zinx/ziface"
)

type ReqLoginMessage struct {
	ServerId int
	Account  int
}

func (login *ReqLoginMessage) GetMessageId() proto.MessageId {
	return proto.MessageId_LOGIN
}

func (login *ReqLoginMessage) DoAction(request ziface.IRequest) error {
	fmt.Println("receive login message")
	user, err := db.FindByAccount(login.Account)
	if err != nil {
		if err.Error() != "mongo: no documents in result" {
			return err
		}
		// 如果未注册，先注册再查找
		err = db.RegisterUser(login.ServerId, login.Account)
		if err != nil {
			return err
		}
		return login.DoAction(request)
	}
	fmt.Println(user)

	//todo: 封装消息返回
	err = request.GetConnection().SendMsg(1, []byte("pong...pong...pong...[FromServer]"))
	if err != nil {
		return err
	}
	return nil
}
