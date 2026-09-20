package internal

import (
	"gameserver/common/session"
)

type roomOfflineHandler interface {
	PlayerOffline(int64) bool
}

func registerCloseAgent(manager roomOfflineHandler) {
	skeleton.RegisterChanRPC("CloseAgent", func(args []interface{}) {
		rpcCloseAgent(manager, args)
	})
}

func rpcCloseAgent(manager roomOfflineHandler, args []interface{}) {
	if len(args) < 2 {
		return
	}
	snapshot, ok := session.FromData(args[1])
	if !ok {
		return
	}
	manager.PlayerOffline(snapshot.PlayerID)
}
