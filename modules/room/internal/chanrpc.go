package internal

import (
	"gameserver/common/models"
	"gameserver/core/gate"
	"gameserver/modules/room/internal/managers"
)

func init() {
	skeleton.RegisterChanRPC("CloseAgent", rpcCloseAgent)
}

func rpcCloseAgent(args []interface{}) {
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
	managers.GetRoomManager().PlayerOffline(user.PlayerId)
}
