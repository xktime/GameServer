package game

import (
	managerplayer "gameserver/modules/game/internal/managers/player"
	"gameserver/modules/game/internal/managers/team"
	"gameserver/modules/match/playerread"
)

type matchPlayerFinder interface {
	GetPlayer(playerID int64) *managerplayer.Player
	GetRandomPlayer(excludedPlayerIDs []int64) *managerplayer.Player
}

type matchTeamFinder interface {
	GetTeamByPlayerId(playerID int64) *team.Team
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
	player := r.players.GetPlayer(playerID)
	if player == nil {
		return playerread.PlayerSnapshot{}, false
	}
	return playerread.PlayerSnapshot{TeamID: player.TeamId}, true
}

func (r matchPlayerReader) FindOnlineTeam(playerID int64) (playerread.TeamSnapshot, bool) {
	teamInfo := r.teams.GetTeamByPlayerId(playerID)
	if teamInfo == nil {
		return playerread.TeamSnapshot{}, false
	}
	return playerread.TeamSnapshot{
		MemberIDs: append([]int64(nil), teamInfo.TeamMembers...),
	}, true
}

func (r matchPlayerReader) FindRandomOnline(excludedPlayerIDs []int64) (int64, bool) {
	player := r.players.GetRandomPlayer(excludedPlayerIDs)
	if player == nil {
		return 0, false
	}
	return player.PlayerId, true
}
