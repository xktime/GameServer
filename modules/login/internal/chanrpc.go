package internal

import (
	"gameserver/core/gate"
	"gameserver/modules/login/internal/handlers"
)

func registerAgentRPC(connections handlers.ConnectManager) {
	skeleton.RegisterChanRPC("NewAgent", rpcNewAgent)
	skeleton.RegisterChanRPC("CloseAgent", func(args []interface{}) {
		rpcCloseAgent(connections, args)
	})
}

func rpcNewAgent(args []interface{}) {
	a := args[0].(gate.Agent)
	_ = a
}

func rpcCloseAgent(connections handlers.ConnectManager, args []interface{}) {
	a := args[0].(gate.Agent)
	if a == nil {
		return
	}
	connections.RemoveClient(a.RemoteAddr().String())
	_ = a
}
