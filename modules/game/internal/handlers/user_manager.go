package handlers

import (
	"gameserver/common/msg/message"
	"gameserver/common/session"
)

type UserManager interface {
	ClosePlayerSession(session.PlayerSession)
	CheckName(string) message.Result
	GetPlayerInfo(int64) (*message.PlayerInfo, bool)
	ModifyName(int64, string) (message.Result, string)
	ModifyAvatarSuffix(int64, string) (message.Result, string)
}
