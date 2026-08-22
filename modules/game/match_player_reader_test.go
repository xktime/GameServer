package game

import (
	managerplayer "gameserver/modules/game/internal/managers/player"
	"gameserver/modules/game/internal/managers/team"
	"testing"
)

type fakeMatchPlayerFinder struct {
	players map[int64]*managerplayer.Player
	random  *managerplayer.Player
}

func (f fakeMatchPlayerFinder) GetPlayer(playerID int64) *managerplayer.Player {
	return f.players[playerID]
}

func (f fakeMatchPlayerFinder) GetRandomPlayer([]int64) *managerplayer.Player {
	return f.random
}

type fakeMatchTeamFinder map[int64]*team.Team

func (f fakeMatchTeamFinder) GetTeamByPlayerId(playerID int64) *team.Team {
	return f[playerID]
}

func TestMatchPlayerReaderCopiesOnlineSnapshots(t *testing.T) {
	player := &managerplayer.Player{PlayerId: 42, TeamId: 7}
	teamInfo := &team.Team{TeamId: 7, TeamMembers: []int64{42, 43}}
	reader := matchPlayerReader{
		players: fakeMatchPlayerFinder{
			players: map[int64]*managerplayer.Player{42: player},
			random:  player,
		},
		teams: fakeMatchTeamFinder{42: teamInfo},
	}

	playerSnapshot, ok := reader.FindOnline(42)
	if !ok || playerSnapshot.TeamID != 7 {
		t.Fatalf("在线 Player 快照 = %#v, %v", playerSnapshot, ok)
	}

	teamSnapshot, ok := reader.FindOnlineTeam(42)
	if !ok || len(teamSnapshot.MemberIDs) != 2 || teamSnapshot.MemberIDs[0] != 42 || teamSnapshot.MemberIDs[1] != 43 {
		t.Fatalf("在线 Team 快照 = %#v, %v", teamSnapshot, ok)
	}
	teamSnapshot.MemberIDs[0] = 99
	if teamInfo.TeamMembers[0] != 42 {
		t.Fatal("修改 TeamSnapshot 不应改写 Game Team")
	}

	randomPlayerID, ok := reader.FindRandomOnline([]int64{1, 2})
	if !ok || randomPlayerID != 42 {
		t.Fatalf("随机在线 Player = %d, %v", randomPlayerID, ok)
	}
}

func TestMatchPlayerReaderRejectsMissingOnlineData(t *testing.T) {
	reader := matchPlayerReader{
		players: fakeMatchPlayerFinder{players: map[int64]*managerplayer.Player{}},
		teams:   fakeMatchTeamFinder{},
	}

	if snapshot, ok := reader.FindOnline(42); ok {
		t.Fatalf("离线 Player 不应返回快照: %#v", snapshot)
	}
	if snapshot, ok := reader.FindOnlineTeam(42); ok {
		t.Fatalf("缺失 Team 不应返回快照: %#v", snapshot)
	}
	if playerID, ok := reader.FindRandomOnline(nil); ok {
		t.Fatalf("无候选 Player 时不应返回 %d", playerID)
	}
}
