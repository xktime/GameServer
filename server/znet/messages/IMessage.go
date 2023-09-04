package messages

type MessageId uint32

const (
	LOGIN MessageId = 1
)

type IMessage interface {
	GetMessageId() MessageId
}

func GetMessage(messageId MessageId) IMessage {
	// todo 双向绑定有点蠢
	switch messageId {
	case LOGIN:
		return &LoginMessage{}
	}
	return nil
}
