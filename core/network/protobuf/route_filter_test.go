package protobuf

import (
	"errors"
	"gameserver/common/msg/message"
	networkmodels "gameserver/core/network/models"
	"testing"
)

func TestRouteFilterRunsBeforeMessageHandler(t *testing.T) {
	processor := NewProcessor()
	processor.Register(&message.C2S_CheckName{})
	handlerCalled := false
	processor.SetHandler(&message.C2S_CheckName{}, func([]interface{}) {
		handlerCalled = true
	})

	blocked := errors.New("blocked")
	routeContext := &struct{}{}
	processor.SetRouteFilter(func(msg interface{}, context interface{}) error {
		if _, ok := msg.(*message.C2S_CheckName); !ok {
			t.Fatalf("过滤器消息 = %T", msg)
		}
		if context != routeContext {
			t.Fatalf("过滤器上下文 = %#v，期望 %#v", context, routeContext)
		}
		return blocked
	})

	err := processor.Route(&networkmodels.MsgWithSeq{MsgData: &message.C2S_CheckName{}}, routeContext)
	if !errors.Is(err, blocked) {
		t.Fatalf("Route 错误 = %v，期望 %v", err, blocked)
	}
	if handlerCalled {
		t.Fatal("过滤器拒绝后不应调用消息 Handler")
	}
}
