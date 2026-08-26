package game

import (
	managerplayer "gameserver/modules/game/internal/managers/player"
	"gameserver/modules/game/internal/managers/team"
	"gameserver/modules/match/playerread"
)

type matchPlayerFinder interface {
	GetPlayerSnapshot(playerID int64) (managerplayer.Snapshot, bool)
}

type matchTeamFinder interface {
	GetTeamByPlayerID(playerID int64) (team.Snapshot, bool)
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

func (r matchPlayerReader) FindOnline(playerID int64) (playerread.PlayerSnapshot, bool) {
	playerSnapshot, found := r.players.GetPlayerSnapshot(playerID)
	if !found {
		return playerread.PlayerSnapshot{}, false
	}
	return playerread.PlayerSnapshot{TeamID: playerSnapshot.TeamID}, true
}

func (r matchPlayerReader) FindOnlineTeam(playerID int64) (playerread.TeamSnapshot, bool) {
	teamSnapshot, found := r.teams.GetTeamByPlayerID(playerID)
	if !found {
		return playerread.TeamSnapshot{}, false
	}
	return playerread.TeamSnapshot{
		MemberIDs: append([]int64(nil), teamSnapshot.MemberIDs...),
	}, true
}
