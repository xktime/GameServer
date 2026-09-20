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
		_ = dispatcher.Call0("NewAgent", a)
	}
	_ = a
}

func rpcCloseAgent(args []interface{}) {
	if len(args) < 2 {
		return
	}
	a := args[0].(gate.Agent)
	snapshot := args[1]
	for _, dispatcher := range Dispatchers {
		dispatcher.Go("CloseAgent", a, snapshot)
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
