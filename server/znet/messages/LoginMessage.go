package messages

import (
	"encoding/json"
	"fmt"
)

type LoginMessage struct {
	ServerId int
	Account  int
}

func (login *LoginMessage) GetMessageId() MessageId {
	return LOGIN
}

func (login *LoginMessage) DoAction(data []byte) error {
	err := json.Unmarshal(data, &login)
	if err != nil {
		return err
	}
	fmt.Println("DoAction", login)
	return nil
}
