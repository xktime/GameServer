package internal

import (
	"gameserver/core/gate"
)

func init() {
	skeleton.RegisterChanRPC("CloseAgent", rpcCloseAgent)
}

func rpcCloseAgent(args []interface{}) {
	a := args[0].(gate.Agent)
	_ = a
}
