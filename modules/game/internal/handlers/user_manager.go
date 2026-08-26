package handlers

import "gameserver/common/msg/message"

type UserManager interface {
	CheckName(string) message.Result
	GetPlayerInfo(int64) (*message.PlayerInfo, bool)
	ModifyName(int64, string) (message.Result, string)
	ModifyAvatarSuffix(int64, string) (message.Result, string)
}
