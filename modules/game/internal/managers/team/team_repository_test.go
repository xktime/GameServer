package team

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"testing"
	"time"
)

type fakeTeamRepository struct {
	loaded     *Team
	loadErr    error
	loadIDs    []int64
	saved      []Snapshot
	deletedIDs []int64
}

func (r *fakeTeamRepository) FindByID(teamID int64) (*Team, error) {
	r.loadIDs = append(r.loadIDs, teamID)
	if r.loaded == nil {
		return nil, r.loadErr
	}
	copyOfTeam := *r.loaded
	copyOfTeam.TeamMembers = append([]int64(nil), r.loaded.TeamMembers...)
	return &copyOfTeam, r.loadErr
}

func (r *fakeTeamRepository) Save(current *Team) error {
	r.saved = append(r.saved, current.snapshot())
	return nil
}

func (r *fakeTeamRepository) DeleteByID(teamID int64) error {
	r.deletedIDs = append(r.deletedIDs, teamID)
	return nil
}

func TestRegistrySnapshotLoadsAndSavesExistingTeamThroughRepository(t *testing.T) {
	repository := &fakeTeamRepository{loaded: &Team{
		TeamId:      7,
		LeaderId:    42,
		TeamMembers: []int64{42, 43},
		RoomId:      "room-1",
	}}
	registry := newTeamRepositoryTestRegistry(t, repository)

	snapshot, err := registry.Snapshot(context.Background(), 7)
	if err != nil {
		t.Fatalf("恢复 Team: %v", err)
	}
	if len(repository.loadIDs) != 1 || repository.loadIDs[0] != 7 {
		t.Fatalf("Repository 加载 ID = %v，期望 [7]", repository.loadIDs)
	}
	if snapshot.TeamID != 7 || snapshot.LeaderID != 42 || snapshot.RoomID != "room-1" || len(snapshot.MemberIDs) != 2 {
		t.Fatalf("恢复后的 Team 快照 = %#v", snapshot)
	}

	instance, err := registry.lookup(context.Background(), 7)
	if err != nil {
		t.Fatalf("查找已恢复 Team: %v", err)
	}
	if err := instance.Ref().Stop(context.Background()); err != nil {
		t.Fatalf("停止已恢复 Team: %v", err)
	}
	if len(repository.saved) != 1 || repository.saved[0].TeamID != 7 || repository.saved[0].RoomID != "room-1" {
		t.Fatalf("Repository 保存 = %#v", repository.saved)
	}
}

func TestRegistryGetOrCreateNewTeamSkipsRepositoryLoad(t *testing.T) {
	repository := &fakeTeamRepository{loadErr: errors.New("load should not run")}
	registry := newTeamRepositoryTestRegistry(t, repository)

	snapshot, err := registry.GetOrCreate(context.Background(), 8, 44)
	if err != nil {
		t.Fatalf("创建 Team: %v", err)
	}
	if len(repository.loadIDs) != 0 {
		t.Fatalf("新 Team 触发了 Repository 加载: %v", repository.loadIDs)
	}
	if snapshot.TeamID != 8 || snapshot.LeaderID != 44 {
		t.Fatalf("新 Team 快照 = %#v", snapshot)
	}
}

func TestRegistrySnapshotPropagatesLoadFailure(t *testing.T) {
	loadFailure := errors.New("load unavailable")
	repository := &fakeTeamRepository{loadErr: loadFailure}
	registry := newTeamRepositoryTestRegistry(t, repository)

	_, err := registry.Snapshot(context.Background(), 9)
	if !errors.Is(err, loadFailure) {
		t.Fatalf("Team 加载错误 = %v，期望 %v", err, loadFailure)
	}
	if _, err := registry.lookup(context.Background(), 9); !errors.Is(err, actor.ErrActorStopped) {
		t.Fatalf("加载失败后 Team 状态 = %v，期望 %v", err, actor.ErrActorStopped)
	}
}

func TestRegistrySnapshotMissingTeamRemainsStopped(t *testing.T) {
	repository := &fakeTeamRepository{}
	registry := newTeamRepositoryTestRegistry(t, repository)

	_, err := registry.Snapshot(context.Background(), 10)
	if !errors.Is(err, actor.ErrActorStopped) {
		t.Fatalf("不存在的 Team = %v，期望 %v", err, actor.ErrActorStopped)
	}
	if _, err := registry.lookup(context.Background(), 10); !errors.Is(err, actor.ErrActorStopped) {
		t.Fatalf("查询不存在 Team 后的状态 = %v，期望 %v", err, actor.ErrActorStopped)
	}
}

func TestLeaveLastMemberDeletesThroughRepository(t *testing.T) {
	repository := &fakeTeamRepository{}
	registry := newTeamRepositoryTestRegistry(t, repository)

	if _, err := registry.GetOrCreate(context.Background(), 11, 45); err != nil {
		t.Fatalf("创建 Team: %v", err)
	}
	if err := registry.Join(context.Background(), 11, 45); err != nil {
		t.Fatalf("加入 Team: %v", err)
	}
	if err := registry.Leave(context.Background(), 11, 45); err != nil {
		t.Fatalf("离开 Team: %v", err)
	}
	if len(repository.deletedIDs) != 1 || repository.deletedIDs[0] != 11 {
		t.Fatalf("Repository 删除 ID = %v，期望 [11]", repository.deletedIDs)
	}
	if len(repository.saved) != 0 {
		t.Fatalf("已删除 Team 仍被保存: %#v", repository.saved)
	}
}

func newTeamRepositoryTestRegistry(t *testing.T, repository teamRepository) *Registry {
	t.Helper()
	system := actor.NewActorSystem(time.Second)
	t.Cleanup(func() {
		if err := system.Stop(context.Background()); err != nil {
			t.Errorf("停止 ActorSystem: %v", err)
		}
	})
	scope, err := system.NewScope("team-repository-test")
	if err != nil {
		t.Fatalf("创建 Scope: %v", err)
	}
	registry, err := newRegistry(scope, repository)
	if err != nil {
		t.Fatalf("创建 Team Registry: %v", err)
	}
	return registry
}
