package managers

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"gameserver/modules/game/internal/managers/player"
	"gameserver/modules/game/internal/managers/team"
	"testing"
	"time"
)

func TestTeamManagerSetRoomProjectionPreservesMissingTeamError(t *testing.T) {
	system := actor.NewActorSystem(time.Second)
	t.Cleanup(func() {
		if err := system.Stop(context.Background()); err != nil {
			t.Errorf("停止 ActorSystem: %v", err)
		}
	})
	scope, err := system.NewScope("team-manager-test")
	if err != nil {
		t.Fatalf("创建 Scope: %v", err)
	}
	teams, err := team.NewRegistryWithRepository(scope, stubTeamRepository{})
	if err != nil {
		t.Fatalf("创建 Team Registry: %v", err)
	}
	players, err := player.NewRegistry(scope, teams)
	if err != nil {
		t.Fatalf("创建 Player Registry: %v", err)
	}
	_, err = teams.PrepareLogin(context.Background(), 7, 42, true)
	if err != nil {
		t.Fatalf("创建 Team: %v", err)
	}
	manager, err := NewTeamManager(players, teams)
	if err != nil {
		t.Fatalf("创建 TeamManager: %v", err)
	}

	if err := manager.SetRoomProjection(context.Background(), 7, "room-1"); err != nil {
		t.Fatalf("设置已有 Team 的投影: %v", err)
	}
	missing := manager.SetRoomProjection(context.Background(), 8, "room-1")
	snapshot, err := teams.Snapshot(context.Background(), 7)
	if err != nil {
		t.Fatalf("获取 Team 快照: %v", err)
	}

	if !errors.Is(missing, actor.ErrActorNotFound) || snapshot.RoomID != "room-1" {
		t.Fatalf("projection results = missing:%v RoomId:%q", missing, snapshot.RoomID)
	}
}
