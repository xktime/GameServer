package game

import (
	managerplayer "gameserver/modules/game/internal/managers/player"
	"gameserver/modules/game/internal/managers/team"
	"testing"
)

type fakeMatchPlayerFinder struct {
	players map[int64]managerplayer.Snapshot
}

func (f fakeMatchPlayerFinder) GetPlayerSnapshot(playerID int64) (managerplayer.Snapshot, bool) {
	snapshot, found := f.players[playerID]
	return snapshot, found
}

type fakeMatchTeamFinder map[int64]team.Snapshot

func (f fakeMatchTeamFinder) GetTeamByPlayerID(playerID int64) (team.Snapshot, bool) {
	snapshot, found := f[playerID]
	return snapshot, found
}

func TestMatchPlayerReaderCopiesOnlineSnapshots(t *testing.T) {
	playerSnapshotData := managerplayer.Snapshot{PlayerID: 42, TeamID: 7}
	teamSnapshotData := team.Snapshot{TeamID: 7, MemberIDs: []int64{42, 43}}
	reader := matchPlayerReader{
		players: fakeMatchPlayerFinder{
			players: map[int64]managerplayer.Snapshot{42: playerSnapshotData},
		},
		teams: fakeMatchTeamFinder{42: teamSnapshotData},
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
	if teamSnapshotData.MemberIDs[0] != 42 {
		t.Fatal("修改 TeamSnapshot 不应改写 Game Team")
	}

}

func TestMatchPlayerReaderRejectsMissingOnlineData(t *testing.T) {
	reader := matchPlayerReader{
		players: fakeMatchPlayerFinder{players: map[int64]managerplayer.Snapshot{}},
		teams:   fakeMatchTeamFinder{},
	}

	if snapshot, ok := reader.FindOnline(42); ok {
		t.Fatalf("离线 Player 不应返回快照: %#v", snapshot)
	}
	if snapshot, ok := reader.FindOnlineTeam(42); ok {
		t.Fatalf("缺失 Team 不应返回快照: %#v", snapshot)
	}
}
