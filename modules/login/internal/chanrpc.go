package internal

import (
	"gameserver/core/gate"
	"gameserver/modules/login/internal/handlers"
)

func registerAgentRPC(connections handlers.ConnectManager) {
	skeleton.RegisterChanRPC("NewAgent", func(args []interface{}) {
		rpcNewAgent(connections, args)
	})
	skeleton.RegisterChanRPC("CloseAgent", func(args []interface{}) {
		rpcCloseAgent(connections, args)
	})
}

func rpcNewAgent(connections handlers.ConnectManager, args []interface{}) {
	if len(args) == 0 {
		return
	}
	a, ok := args[0].(gate.ConnectionAgent)
	if !ok {
		return
	}
	connections.Register(a)
}

func rpcCloseAgent(connections handlers.ConnectManager, args []interface{}) {
	if len(args) == 0 {
		return
	}
	a, ok := args[0].(gate.ConnectionAgent)
	if !ok {
		return
	}
	connections.RemoveClient(a.ConnectionID())
}
