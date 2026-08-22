package game

import (
	managerplayer "gameserver/modules/game/internal/managers/player"
	playermodel "gameserver/modules/game/internal/models/player"
	"testing"
)

type fakeOnlinePlayerFinder map[int64]*managerplayer.Player

func (f fakeOnlinePlayerFinder) GetPlayer(playerID int64) *managerplayer.Player {
	return f[playerID]
}

func TestRankPlayerReaderCopiesOnlinePlayerSnapshot(t *testing.T) {
	reader := rankPlayerReader{players: fakeOnlinePlayerFinder{
		42: {
			PlayerId: 42,
			PlayerInfo: &playermodel.PlayerInfo{
				PlayerId:     42,
				PlayerName:   "测试玩家",
				AvatarSuffix: "/avatar.png",
				Level:        12,
			},
		},
	}}

	snapshot, ok := reader.FindOnline(42)

	if !ok {
		t.Fatal("在线 Player 应返回快照")
	}
	if snapshot.Name != "测试玩家" {
		t.Fatalf("玩家名称 = %q，期望 %q", snapshot.Name, "测试玩家")
	}
	if snapshot.AvatarURL != "https://rank-server.oss-cn-hangzhou.aliyuncs.com/avatar/42/avatar.png" {
		t.Fatalf("头像 URL = %q", snapshot.AvatarURL)
	}
	if snapshot.Level != 12 {
		t.Fatalf("玩家等级 = %d，期望 12", snapshot.Level)
	}
}

func TestRankPlayerReaderRejectsPlayerWithoutInfo(t *testing.T) {
	reader := rankPlayerReader{players: fakeOnlinePlayerFinder{
		42: {PlayerId: 42},
	}}

	if snapshot, ok := reader.FindOnline(42); ok {
		t.Fatalf("缺少 PlayerInfo 的 Player 不应返回快照: %#v", snapshot)
	}
}
