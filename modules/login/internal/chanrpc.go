package internal

import (
	"gameserver/core/gate"
	"gameserver/modules/login/internal/managers"
)

func init() {
	skeleton.RegisterChanRPC("NewAgent", rpcNewAgent)
	skeleton.RegisterChanRPC("CloseAgent", rpcCloseAgent)
}

func rpcNewAgent(args []interface{}) {
	a := args[0].(gate.Agent)
	_ = a
}

func rpcCloseAgent(args []interface{}) {
	a := args[0].(gate.Agent)
	if a == nil {
		return
	}
	managers.GetConnectManager().RemoveClient(a.RemoteAddr().String())
	_ = a
}
