package messages

import (
	"GameServer/server/znet/messages/proto"
	"encoding/json"
	"fmt"
	"github.com/aceld/zinx/ziface"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type IMessage interface {
	GetMessageId() uint32
	GetProtoMessage() ProtoMessage
}

type ProtoMessage interface {
	ProtoReflect() protoreflect.Message
}

func NewC2SMessage(messageId proto.C2SMessageId, message ProtoMessage) IMessage {
	return &C2SMessage{
		MessageId: messageId,
		Message:   message,
	}
}

func NewS2CMessage(messageId proto.S2CMessageId, message ProtoMessage) IMessage {
	return &S2CMessage{
		MessageId: messageId,
		Message:   message,
	}
}

// SendMessage 消息发送
func SendMessage(conn ziface.IConnection, message IMessage) error {
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
