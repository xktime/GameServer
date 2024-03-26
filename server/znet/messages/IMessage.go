package messages

import (
	"encoding/json"
	"fmt"
	"github.com/aceld/zinx/ziface"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Message interface {
	GetMessageId() uint32
	GetProtoMessage() ProtoMessage
}

type ProtoMessage interface {
	ProtoReflect() protoreflect.Message
}

// SendMessage 消息发送
func SendMessage(conn ziface.IConnection, message Message) error {
	messageId := message.GetMessageId()
	if messageId == 0 {
		return fmt.Errorf("no such messageId:%d", messageId)
	}

	protoMessage := message.GetProtoMessage()
	if protoMessage == nil {
		return fmt.Errorf("no such message:%q", protoMessage)
	}

	output, _ := json.Marshal(protoMessage)
	err := conn.SendMsg(messageId, output)
	if err != nil {
		return err
	}
	return nil
}
