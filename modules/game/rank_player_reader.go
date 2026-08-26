package game

import (
	managerplayer "gameserver/modules/game/internal/managers/player"
	"gameserver/modules/rank/playerread"
)

type onlinePlayerFinder interface {
	GetPlayerSnapshot(playerID int64) (managerplayer.Snapshot, bool)
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
	playerSnapshot, found := r.players.GetPlayerSnapshot(playerID)
	if !found || playerSnapshot.PlayerInfo == nil {
		return playerread.PlayerSnapshot{}, false
	}

	return playerread.PlayerSnapshot{
		Name:      playerSnapshot.PlayerInfo.PlayerName,
		AvatarURL: playerSnapshot.PlayerInfo.GetAvatarURL(),
		Level:     playerSnapshot.PlayerInfo.Level,
	}, true
}
