package messages

import (
	"GameServer/server/common/Tools"
	"GameServer/server/znet/messages/proto"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

type IMessage interface {
	GetMessageId() proto.MessageId
	DoAction() error
}

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

// Load todo 用反射方式加载或自注册解决双向绑定
func _() {
	pwd, _ := os.Getwd()
	structList := Tools.GetStructListByDir(pwd)
	// todo 根据返回结构体初始化对象
	// todo 反射messageId类型，缓存进map
	fmt.Print(structList)
}
