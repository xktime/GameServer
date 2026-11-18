package team

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"sync"
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
	registry, err := NewRegistryWithRepository(scope, &fakeTeamRepository{})
	if err != nil {
		t.Fatalf("创建 Team Registry: %v", err)
	}
	_, err = registry.PrepareLogin(context.Background(), 7, 42, true)
	if err != nil {
		t.Fatalf("创建 Team: %v", err)
	}

	if err := registry.SetRoomProjection(context.Background(), 7, "room-1"); err != nil {
		t.Fatalf("首次设置房间投影: %v", err)
	}
	if err := registry.SetRoomProjection(context.Background(), 7, "room-1"); err != nil {
		t.Fatalf("重复设置房间投影: %v", err)
	}
	if err := registry.SetRoomProjection(context.Background(), 7, ""); err != nil {
		t.Fatalf("清除房间投影: %v", err)
	}
	snapshot, err := registry.Snapshot(context.Background(), 7)
	if err != nil {
		t.Fatalf("获取 Team 快照: %v", err)
	}

	if snapshot.RoomID != "" {
		t.Fatalf("RoomID = %q，期望空值", snapshot.RoomID)
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
	registry, err := NewRegistryWithRepository(scope, &fakeTeamRepository{})
	if err != nil {
		t.Fatalf("创建 Team Registry: %v", err)
	}
	_, err = registry.PrepareLogin(context.Background(), 7, 42, true)
	if err != nil {
		t.Fatalf("创建 Team: %v", err)
	}
	if err := registry.Leave(context.Background(), 7, 42); err != nil {
		t.Fatalf("离开 Team: %v", err)
	}
	if _, err := registry.Snapshot(context.Background(), 7); !errors.Is(err, actor.ErrActorNotFound) {
		t.Fatalf("查找空 Team: got %v, want %v", err, actor.ErrActorNotFound)
	}
}

func TestQueuedJoinCannotReviveTeamAfterLastMemberLeaves(t *testing.T) {
	system := actor.NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	scope, _ := system.NewScope("team-queued-join")
	repository := &fakeTeamRepository{}
	registry, _ := NewRegistryWithRepository(scope, repository)
	if _, err := registry.PrepareLogin(context.Background(), 7, 42, true); err != nil {
		t.Fatal(err)
	}
	instance, _ := registry.lookup(context.Background(), 7)
	started, release := make(chan struct{}), make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseOnce)
	if err := actor.Tell(context.Background(), instance.Ref(), func(actor.Context) error {
		close(started)
		<-release
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	waitDepth := func(depth int64) {
		t.Helper()
		deadline := time.Now().Add(time.Second)
		for system.Metrics().QueueDepth != depth {
			if time.Now().After(deadline) {
				t.Fatalf("queue never reached depth %d", depth)
			}
			time.Sleep(time.Millisecond)
		}
	}
	left, joined := make(chan error, 1), make(chan error, 1)
	go func() { left <- registry.Leave(context.Background(), 7, 42) }()
	waitDepth(1)
	go func() { joined <- registry.Join(context.Background(), 7, 43) }()
	waitDepth(2)
	releaseOnce()
	if err := <-left; err != nil {
		t.Fatalf("last member Leave = %v", err)
	}
	if err := <-joined; !errors.Is(err, actor.ErrActorStopping) {
		t.Fatalf("queued Join = %v; want rejected domain operation", err)
	}
	if len(repository.deletedIDs) != 1 || len(repository.saved) != 0 {
		t.Fatalf("deleted Team was revived: deleted=%v saved=%v", repository.deletedIDs, repository.saved)
	}
}
