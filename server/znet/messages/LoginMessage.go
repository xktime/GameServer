package messages

import (
	"GameServer/server/db"
	"fmt"
)

type ReqLoginMessage struct {
	ServerId int
	Account  int
}

func (login *ReqLoginMessage) GetMessageId() MessageId {
	return LOGIN
}

func (login *ReqLoginMessage) DoAction() error {
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
		return login.DoAction()
	}

	fmt.Println(user)
	return nil
}
