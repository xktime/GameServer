package managers

import (
	"context"
	"gameserver/common/base/actor"
	"gameserver/modules/game/internal/managers/player"
	"gameserver/modules/game/internal/managers/team"
	"testing"
	"time"
)

func TestTeamManagerSetRoomProjectionReturnsWhetherTeamExists(t *testing.T) {
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
	teams, err := team.NewRegistry(scope)
	if err != nil {
		t.Fatalf("创建 Team Registry: %v", err)
	}
	players, err := player.NewRegistry(scope, teams)
	if err != nil {
		t.Fatalf("创建 Player Registry: %v", err)
	}
	_, err = teams.GetOrCreate(context.Background(), 7, 42)
	if err != nil {
		t.Fatalf("创建 Team: %v", err)
	}
	manager, err := NewTeamManager(context.Background(), scope, players, teams)
	if err != nil {
		t.Fatalf("创建 TeamManager: %v", err)
	}

	applied := manager.SetRoomProjection(7, "room-1")
	missing := manager.SetRoomProjection(8, "room-1")
	snapshot, err := teams.Snapshot(context.Background(), 7)
	if err != nil {
		t.Fatalf("获取 Team 快照: %v", err)
	}

	if !applied || missing || snapshot.RoomID != "room-1" {
		t.Fatalf("projection results = applied:%t missing:%t RoomId:%q", applied, missing, snapshot.RoomID)
	}
}
