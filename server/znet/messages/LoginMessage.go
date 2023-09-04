package messages

type LoginMessage struct {
	ServerId int
	Account  int
}

func (login *LoginMessage) GetMessageId() MessageId {
	return LOGIN
}
