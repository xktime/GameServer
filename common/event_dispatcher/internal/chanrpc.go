package internal

import (
	"gameserver/core/chanrpc"
	"gameserver/core/gate"
)

var Dispatchers []*chanrpc.Server

func init() {
	skeleton.RegisterChanRPC("NewAgent", rpcNewAgent)
	skeleton.RegisterChanRPC("CloseAgent", rpcCloseAgent)
	skeleton.RegisterChanRPC("OnGetItem", rpcOnGetItem)
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

func rpcOnGetItem(args []interface{}) {
	playerId := args[0].(int64)
	itemId := args[1].(int32)
	count := args[2].(int32)
	for _, dispatcher := range Dispatchers {
		dispatcher.Go("OnGetItem", playerId, itemId, count)
	}
}
