package messages

import (
	"GameServer/server/common/Tools"
	"fmt"
)

type MessageId uint32

const (
	LOGIN MessageId = 1
)

type IMessage interface {
	GetMessageId() MessageId
	DoAction(data []byte) error
}

func DoAction(messageId MessageId, data []byte) error {
	// todo 双向绑定有点蠢
	var message IMessage
	switch messageId {
	case LOGIN:
		message = &LoginMessage{}
		break
	default:
		return nil
	}
	return message.DoAction(data)
}

func Load() {
	structList := Tools.GetStructListByDir("./server/znet/messages/")
	fmt.Print(structList)
}
