package team

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"testing"
	"time"
)

func TestSetRoomProjectionSynchronouslyAppliesLatestDesiredValue(t *testing.T) {
	system := actor.NewActorSystem(time.Second)
	t.Cleanup(func() {
		if err := system.Stop(context.Background()); err != nil {
			t.Errorf("停止 ActorSystem: %v", err)
		}
	})
	scope, err := system.NewScope("team-test")
	if err != nil {
		t.Fatalf("创建 Scope: %v", err)
	}
	registry, err := NewRegistry(scope)
	if err != nil {
		t.Fatalf("创建 Team Registry: %v", err)
	}
	_, err = registry.GetOrCreate(context.Background(), 7, 42)
	if err != nil {
		t.Fatalf("创建 Team: %v", err)
	}

	first, err := registry.SetRoomProjection(context.Background(), 7, "room-1")
	if err != nil {
		t.Fatalf("首次设置房间投影: %v", err)
	}
	repeated, err := registry.SetRoomProjection(context.Background(), 7, "room-1")
	if err != nil {
		t.Fatalf("重复设置房间投影: %v", err)
	}
	cleared, err := registry.SetRoomProjection(context.Background(), 7, "")
	if err != nil {
		t.Fatalf("清除房间投影: %v", err)
	}
	snapshot, err := registry.Snapshot(context.Background(), 7)
	if err != nil {
		t.Fatalf("获取 Team 快照: %v", err)
	}

	if !first || !repeated || !cleared || snapshot.RoomID != "" {
		t.Fatalf("projection results = first:%t repeated:%t cleared:%t RoomId:%q", first, repeated, cleared, snapshot.RoomID)
	}
}

func TestLeaveLastMemberStopsAndRemovesTeam(t *testing.T) {
	system := actor.NewActorSystem(time.Second)
	t.Cleanup(func() {
		if err := system.Stop(context.Background()); err != nil {
			t.Errorf("停止 ActorSystem: %v", err)
		}
	})
	scope, err := system.NewScope("team-leave-test")
	if err != nil {
		t.Fatalf("创建 Scope: %v", err)
	}
	registry, err := NewRegistry(scope)
	if err != nil {
		t.Fatalf("创建 Team Registry: %v", err)
	}
	_, err = registry.GetOrCreate(context.Background(), 7, 42)
	if err != nil {
		t.Fatalf("创建 Team: %v", err)
	}
	if err := registry.Join(context.Background(), 7, 42); err != nil {
		t.Fatalf("加入 Team: %v", err)
	}
	if err := registry.Leave(context.Background(), 7, 42); err != nil {
		t.Fatalf("离开 Team: %v", err)
	}
	if _, err := registry.Snapshot(context.Background(), 7); !errors.Is(err, actor.ErrActorStopped) {
		t.Fatalf("查找空 Team: got %v, want %v", err, actor.ErrActorStopped)
	}
}
