package event_dispatcher

import (
	"gameserver/common/event_dispatcher/internal"
	"gameserver/core/chanrpc"
)

var (
	Module  = new(internal.Module)
	ChanRPC = internal.ChanRPC
)

func RegisterDispatcher(dispatcher *chanrpc.Server) {
	internal.Dispatchers = append(internal.Dispatchers, dispatcher)
}
