package messages

import (
	"GameServer/server/znet/messages/proto"
	"encoding/json"
	"fmt"
	"github.com/aceld/zinx/ziface"
)

type IMessage interface {
	GetMessageId() uint32
	GetMessage() interface{}
}

// todo: message需要一个基类
func NewC2SMessage(messageId proto.C2SMessageId, message interface{}) IMessage {
	return &C2SMessage{
		MessageId: messageId,
		Message:   message,
	}
}

func NewS2CMessage(messageId proto.S2CMessageId, message interface{}) IMessage {
	return &S2CMessage{
		MessageId: messageId,
		Message:   message,
	}
}

// SendMessage 消息发送
func SendMessage(conn ziface.IConnection, imessage IMessage) error {
	messageId := imessage.GetMessageId()
	if messageId == 0 {
		return fmt.Errorf("no such messageId:%d", messageId)
	}

	message := imessage.GetMessage()
	if message == nil {
		return fmt.Errorf("no such message:%q", message)
	}

	output, _ := json.Marshal(message)
	err := conn.SendMsg(messageId, output)
	if err != nil {
		return err
	}
	return nil
}
