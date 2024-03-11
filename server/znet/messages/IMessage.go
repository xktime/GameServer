package messages

import (
	"GameServer/server/znet/messages/proto"
	"encoding/json"
	"errors"
)

type IMessage interface {
	GetMessageId() proto.MessageId
	DoAction() error
}

// DoAction todo: messageId和data耦合性太强了（方案：GetMessageId要非重复；一次配置，不需要getMessage配一次，结构体内也配一次）
func DoAction(messageId proto.MessageId, data []byte) error {
	var message = getMessage(messageId)
	if message == nil {
		return errors.New("no such message")
	}
	err := json.Unmarshal(data, &message)
	if err != nil {
		return err
	}
	return message.DoAction()
}

func getMessage(messageId proto.MessageId) IMessage {
	switch messageId {
	case proto.MessageId_LOGIN:
		return &ReqLoginMessage{}
	default:
		return nil
	}
}
