package internal

import (
	"gameserver/core/chanrpc"
	"gameserver/core/gate"
)

var Dispatchers []*chanrpc.Server

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
	for _, dispatcher := range Dispatchers {
		dispatcher.Go("CloseAgent", a)
	}
	_ = a
}
