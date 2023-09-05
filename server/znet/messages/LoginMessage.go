package messages

import (
	"fmt"
)

type LoginMessage struct {
	ServerId int
	Account  int
}

func (login *LoginMessage) GetMessageId() MessageId {
	return LOGIN
}

func (login *LoginMessage) DoAction() error {
	fmt.Println("DoAction", login)
	return nil
}
