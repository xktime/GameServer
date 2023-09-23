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
		db.RegisterUser(login.ServerId, login.Account)
		return err
	}
	fmt.Println(user)
	return nil
}
