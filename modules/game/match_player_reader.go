package game

import (
	"context"
	managerplayer "gameserver/modules/game/internal/managers/player"
	"gameserver/modules/game/internal/managers/team"
	"gameserver/modules/match/playerread"
)

type matchPlayerFinder interface {
	GetPlayerSnapshot(ctx context.Context, playerID int64) (managerplayer.Snapshot, bool, error)
}

type matchTeamFinder interface {
	GetTeamByPlayerID(ctx context.Context, playerID int64) (team.Snapshot, bool, error)
}

type matchPlayerReader struct {
	players matchPlayerFinder
	teams   matchTeamFinder
}

var _ playerread.PlayerReader = matchPlayerReader{}

func NewMatchPlayerReader() playerread.PlayerReader {
	if External.UserManager == nil || External.TeamManager == nil {
		panic("game: NewMatchPlayerReader called before GameExternal.InitExternal")
	}
	return matchPlayerReader{
		players: External.UserManager,
		teams:   External.TeamManager,
	}
}

func (r matchPlayerReader) FindOnline(ctx context.Context, playerID int64) (playerread.PlayerSnapshot, bool, error) {
	playerSnapshot, found, err := r.players.GetPlayerSnapshot(ctx, playerID)
	if err != nil || !found {
		return playerread.PlayerSnapshot{}, false, err
	}
	return playerread.PlayerSnapshot{TeamID: playerSnapshot.TeamID}, true, nil
}

func (r matchPlayerReader) FindOnlineTeam(ctx context.Context, playerID int64) (playerread.TeamSnapshot, bool, error) {
	teamSnapshot, found, err := r.teams.GetTeamByPlayerID(ctx, playerID)
	if err != nil || !found {
		return playerread.TeamSnapshot{}, false, err
	}
	return playerread.TeamSnapshot{
		TeamID:    teamSnapshot.TeamID,
		MemberIDs: append([]int64(nil), teamSnapshot.MemberIDs...),
	}, true, nil
}
