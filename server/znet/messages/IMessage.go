package messages

import (
	"GameServer/server/znet/messages/proto"
	"encoding/json"
	"errors"
	"github.com/aceld/zinx/ziface"
)

type IMessage interface {
	GetMessageId() proto.MessageId
	DoAction(request ziface.IRequest) error
}

// DoAction todo: messageId和data耦合性太强了（方案：GetMessageId要非重复；一次配置，不需要getMessage配一次，结构体内也配一次）
func DoAction(request ziface.IRequest) error {
	messageId := proto.MessageId(request.GetMsgID())
	data := request.GetData()
	var message = getMessage(messageId)
	if message == nil {
		return errors.New("no such message")
	}
	err := json.Unmarshal(data, &message)
	if err != nil {
		return err
	}
	return message.DoAction(request)
}

// SendMessage 消息发送：客户端=》服务器
func SendMessage(conn ziface.IConnection, message IMessage) error {
	output, _ := json.Marshal(&message)
	err := conn.SendMsg(uint32(message.GetMessageId()), output)
	if err != nil {
		return err
	}
	return nil
}

func getMessage(messageId proto.MessageId) IMessage {
	switch messageId {
	case proto.MessageId_LOGIN:
		return &ReqLoginMessage{}
	default:
		return nil
	}
}
