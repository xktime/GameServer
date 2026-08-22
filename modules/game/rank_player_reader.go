package game

import (
	managerplayer "gameserver/modules/game/internal/managers/player"
	"gameserver/modules/rank/playerread"
)

type onlinePlayerFinder interface {
	GetPlayer(playerID int64) *managerplayer.Player
}

type rankPlayerReader struct {
	players onlinePlayerFinder
}

var _ playerread.PlayerReader = rankPlayerReader{}

func NewRankPlayerReader() playerread.PlayerReader {
	if External.UserManager == nil {
		panic("game: NewRankPlayerReader called before GameExternal.InitExternal")
	}
	return rankPlayerReader{players: External.UserManager}
}

func (r rankPlayerReader) FindOnline(playerID int64) (playerread.PlayerSnapshot, bool) {
	player := r.players.GetPlayer(playerID)
	if player == nil || player.PlayerInfo == nil {
		return playerread.PlayerSnapshot{}, false
	}

	return playerread.PlayerSnapshot{
		Name:      player.PlayerInfo.PlayerName,
		AvatarURL: player.PlayerInfo.GetAvatarURL(),
		Level:     player.PlayerInfo.Level,
	}, true
}
