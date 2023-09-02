package messages

const (
	LOGIN = 1
)

type IMessage interface {
	GetMessageId() uint32
}

func GetMessage(messageId uint32) IMessage {
	// todo 双向绑定有点蠢
	switch messageId {
	case LOGIN:
		return &LoginMessage{}
	}
	return nil
}
