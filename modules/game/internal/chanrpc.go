package internal

import (
	"gameserver/common/models"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game/internal/managers"
)

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
	user := a.UserData()
	if user != nil {
		log.Debug("断开链接 %v", user)
		managers.GetUserManager().UserOffline(user.(models.User))
	}
	_ = a
}
