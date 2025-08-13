package internal

import (
	"gameserver/common/models"
	"gameserver/core/gate"
)

func init() {
	skeleton.RegisterChanRPC("CloseAgent", rpcCloseAgent)
}

func rpcCloseAgent(args []interface{}) {
	a := args[0].(gate.Agent)
	playerId := a.UserData().(models.User).PlayerId

	// TODO: 实现rank模块的玩家离线逻辑
	// 例如：更新排行榜数据、清理缓存等

	_ = a
	_ = playerId
}
