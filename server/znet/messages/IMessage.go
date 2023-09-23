package messages

import (
	"GameServer/server/common/Tools"
	"encoding/json"
	"fmt"
)

type MessageId uint32

const (
	LOGIN MessageId = 1
)

type IMessage interface {
	GetMessageId() MessageId
	DoAction() error
}

func DoAction(messageId MessageId, data []byte) error {
	var message IMessage
	switch messageId {
	case LOGIN:
		message = &ReqLoginMessage{}
		break
	default:
		return nil
	}
	err := json.Unmarshal(data, &message)
	if err != nil {
		return err
	}
	return message.DoAction()
}

// Load todo 用反射方式加载或自注册解决双向绑定
func Load() {
	structList := Tools.GetStructListByDir("./server/znet/messages/")
	fmt.Print(structList)
}
