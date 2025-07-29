package game

import (
	"gameserver/modules/game/internal"
	"gameserver/modules/game/managers"
)

var (
	Module  = new(internal.Module)
	ChanRPC = internal.ChanRPC
)
var UserManager *managers.UserManager
