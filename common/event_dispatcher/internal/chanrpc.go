package internal

import (
	"gameserver/core/chanrpc"
	"gameserver/core/gate"
)

var Dispatchers []*chanrpc.Server

func init() {
	skeleton.RegisterChanRPC("NewAgent", rpcNewAgent)
	skeleton.RegisterChanRPC("CloseAgent", rpcCloseAgent)
	skeleton.RegisterChanRPC("OnCrossDay", rpcOnCrossDay)
}

func rpcNewAgent(args []interface{}) {
	a := args[0].(gate.Agent)
	for _, dispatcher := range Dispatchers {
		dispatcher.Go("NewAgent", a)
	}
	_ = a
}

func rpcCloseAgent(args []interface{}) {
	a := args[0].(gate.Agent)
	for _, dispatcher := range Dispatchers {
		dispatcher.Go("CloseAgent", a)
	}
	_ = a
}

func rpcOnCrossDay(args []interface{}) {
	timestamp := args[0].(int64)
	for _, dispatcher := range Dispatchers {
		dispatcher.Go("OnCrossDay", timestamp)
	}
	_ = timestamp
}
