package messages

import (
	"GameServer/server/common/Tools"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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

func getMessage(messageId MessageId) IMessage {
	switch messageId {
	case LOGIN:
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
