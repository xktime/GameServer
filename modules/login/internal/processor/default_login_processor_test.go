package processor_test

import (
	"context"
	"testing"

	"gameserver/common/msg/message"
	"gameserver/conf"
	"gameserver/modules/login/internal/processor"
)

func TestDefaultLoginIsRejectedWhenDisabled(t *testing.T) {
	debugEnabled := conf.Server.Debug.Enabled
	allowDefaultLogin := conf.Server.Debug.AllowDefaultLogin
	conf.Server.Debug.Enabled = true
	conf.Server.Debug.AllowDefaultLogin = false
	t.Cleanup(func() {
		conf.Server.Debug.Enabled = debugEnabled
		conf.Server.Debug.AllowDefaultLogin = allowDefaultLogin
	})

	response := processor.NewDefaultLoginProcessor().ReqLogin(nil, context.Background(), &message.C2S_Login{
		LoginType: message.LoginType_None,
	})

	if response == nil {
		t.Fatal("ReqLogin() response is nil")
	}
	if response.ErrCode == 0 {
		t.Fatal("ReqLogin() accepted default login while disabled")
	}
}

func TestDefaultLoginRejectsUnknownTypeWhenEnabled(t *testing.T) {
	allowDefaultLogin := conf.Server.Debug.AllowDefaultLogin
	conf.Server.Debug.AllowDefaultLogin = true
	t.Cleanup(func() {
		conf.Server.Debug.AllowDefaultLogin = allowDefaultLogin
	})

	response := processor.NewDefaultLoginProcessor().ReqLogin(nil, context.Background(), &message.C2S_Login{
		LoginType: message.LoginType(99),
	})

	if response == nil {
		t.Fatal("ReqLogin() response is nil")
	}
	if response.ErrCode == 0 {
		t.Fatal("ReqLogin() accepted an unknown login type")
	}
}
