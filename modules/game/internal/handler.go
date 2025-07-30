package internal

import (
	"reflect"

	"google.golang.org/protobuf/proto"
)

func InitHandler() {
	// 向当前模块（game 模块）注册 Person 消息的消息处理函数 handleTest
}

func handler(m proto.Message, h interface{}) {
	skeleton.RegisterChanRPC(reflect.TypeOf(m), h)
}
