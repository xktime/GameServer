package messages

import (
	"encoding/json"
	"github.com/aceld/zinx/ziface"
)

// todo: 消息接口封装
// SendMessage 消息发送
func SendMessage(conn ziface.IConnection, messageId uint32, message interface{}) error {
	output, _ := json.Marshal(&message)
	err := conn.SendMsg(messageId, output)
	if err != nil {
		return err
	}
	return nil
}
