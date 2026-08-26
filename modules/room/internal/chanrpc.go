package internal

import (
	"gameserver/common/models"
	"gameserver/core/gate"
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
	if len(args) == 0 {
		return
	}
	agent, ok := args[0].(gate.Agent)
	if !ok {
		return
	}
	user, ok := agent.UserData().(models.User)
	if !ok {
		return
	}
	manager.PlayerOffline(user.PlayerId)
}
