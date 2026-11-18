package game

import (
	"context"
	managerplayer "gameserver/modules/game/internal/managers/player"
	"gameserver/modules/rank/playerread"
)

type onlinePlayerFinder interface {
	GetPlayerSnapshot(ctx context.Context, playerID int64) (managerplayer.Snapshot, bool, error)
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

func (r rankPlayerReader) FindOnline(ctx context.Context, playerID int64) (playerread.PlayerSnapshot, bool, error) {
	playerSnapshot, found, err := r.players.GetPlayerSnapshot(ctx, playerID)
	if err != nil || !found || playerSnapshot.PlayerInfo == nil {
		return playerread.PlayerSnapshot{}, false, err
	}

	return playerread.PlayerSnapshot{
		Name:      playerSnapshot.PlayerInfo.PlayerName,
		AvatarURL: playerSnapshot.PlayerInfo.GetAvatarURL(),
		Level:     playerSnapshot.PlayerInfo.Level,
	}, true, nil
}
