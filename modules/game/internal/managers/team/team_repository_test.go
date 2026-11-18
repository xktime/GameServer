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
	saveErr    error
	loadIDs    []int64
	saved      []Snapshot
	deletedIDs []int64
}

func (r *fakeTeamRepository) FindByID(ctx context.Context, teamID int64) (*Team, error) {
	r.loadIDs = append(r.loadIDs, teamID)
	if r.loaded == nil {
		return nil, r.loadErr
	}
	copyOfTeam := *r.loaded
	copyOfTeam.TeamMembers = append([]int64(nil), r.loaded.TeamMembers...)
	return &copyOfTeam, r.loadErr
}

func (r *fakeTeamRepository) Save(ctx context.Context, current *Team) error {
	r.saved = append(r.saved, current.snapshot())
	return r.saveErr
}

func (r *fakeTeamRepository) DeleteByID(ctx context.Context, teamID int64) error {
	r.deletedIDs = append(r.deletedIDs, teamID)
	return nil
}

func TestPrepareLoginLoadsAndSavesExistingTeamThroughRepository(t *testing.T) {
	repository := &fakeTeamRepository{loaded: &Team{
		TeamId:      7,
		LeaderId:    42,
		TeamMembers: []int64{42, 43},
		RoomId:      "room-1",
	}}
	registry := newTeamRepositoryTestRegistry(t, repository)

	prepared, err := registry.PrepareLogin(context.Background(), 7, 42, false)
	if err != nil {
		t.Fatalf("恢复 Team: %v", err)
	}
	if prepared.TeamID() != 7 {
		t.Fatalf("登录准备的 TeamID = %d，期望 7", prepared.TeamID())
	}
	snapshot, err := registry.Snapshot(context.Background(), 7)
	if err != nil {
		t.Fatalf("获取 Team 快照: %v", err)
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

func TestPrepareLoginNewTeamSkipsRepositoryLoad(t *testing.T) {
	repository := &fakeTeamRepository{loadErr: errors.New("load should not run")}
	registry := newTeamRepositoryTestRegistry(t, repository)

	_, err := registry.PrepareLogin(context.Background(), 8, 44, true)
	if err != nil {
		t.Fatalf("创建 Team: %v", err)
	}
	if len(repository.loadIDs) != 0 {
		t.Fatalf("新 Team 触发了 Repository 加载: %v", repository.loadIDs)
	}
	snapshot, err := registry.Snapshot(context.Background(), 8)
	if err != nil {
		t.Fatalf("获取 Team 快照: %v", err)
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
	if _, err := registry.lookup(context.Background(), 9); !errors.Is(err, actor.ErrActorNotFound) {
		t.Fatalf("加载失败后 Team 状态 = %v，期望 %v", err, actor.ErrActorNotFound)
	}
}

func TestOfflineSnapshotDoesNotActivateOrSaveTeam(t *testing.T) {
	repository := &fakeTeamRepository{loaded: &Team{TeamId: 7, TeamMembers: []int64{42}, RoomId: "stored-room"}}
	registry := newTeamRepositoryTestRegistry(t, repository)
	snapshot, err := registry.Snapshot(context.Background(), 7)
	if err != nil || snapshot.TeamID != 7 || snapshot.RoomID != "stored-room" {
		t.Fatalf("offline snapshot = %#v, %v", snapshot, err)
	}
	snapshot.MemberIDs[0] = 99
	if repository.loaded.TeamMembers[0] != 42 {
		t.Fatal("snapshot shares persisted member storage")
	}
	if _, err := registry.lookup(context.Background(), 7); !errors.Is(err, actor.ErrActorNotFound) {
		t.Fatalf("snapshot activated a Team: %v", err)
	}
	if err := registry.Stop(context.Background(), 7); err != nil || len(repository.saved) != 0 {
		t.Fatalf("offline observation triggered persistence: %v, %#v", err, repository.saved)
	}
}

func TestSnapshotDoesNotFallBackAfterLivePersistenceFails(t *testing.T) {
	failure := errors.New("save unavailable")
	repository := &fakeTeamRepository{loaded: &Team{TeamId: 7, RoomId: "stale-room"}, saveErr: failure}
	registry := newTeamRepositoryTestRegistry(t, repository)
	t.Cleanup(func() { repository.saveErr = nil })
	if _, err := registry.PrepareLogin(context.Background(), 7, 42, true); err != nil {
		t.Fatal(err)
	}
	if err := registry.Stop(context.Background(), 7); !errors.Is(err, failure) {
		t.Fatalf("Stop = %v", err)
	}
	if _, err := registry.Snapshot(context.Background(), 7); !errors.Is(err, actor.ErrStopFailed) {
		t.Fatalf("failed live snapshot = %v", err)
	}
	if len(repository.loadIDs) != 0 {
		t.Fatalf("failed live Actor fell back to storage: %v", repository.loadIDs)
	}
	repository.saveErr = nil
	if err := registry.Stop(context.Background(), 7); err != nil || len(repository.saved) != 2 {
		t.Fatalf("retry = %v, saves = %d", err, len(repository.saved))
	}
}

func TestRegistrySnapshotMissingTeamRemainsStopped(t *testing.T) {
	repository := &fakeTeamRepository{}
	registry := newTeamRepositoryTestRegistry(t, repository)

	_, err := registry.Snapshot(context.Background(), 10)
	if !errors.Is(err, actor.ErrActorNotFound) {
		t.Fatalf("不存在的 Team = %v，期望 %v", err, actor.ErrActorNotFound)
	}
	if _, err := registry.lookup(context.Background(), 10); !errors.Is(err, actor.ErrActorNotFound) {
		t.Fatalf("查询不存在 Team 后的状态 = %v，期望 %v", err, actor.ErrActorNotFound)
	}
}

func TestLeaveLastMemberDeletesThroughRepository(t *testing.T) {
	repository := &fakeTeamRepository{}
	registry := newTeamRepositoryTestRegistry(t, repository)

	if _, err := registry.PrepareLogin(context.Background(), 11, 45, true); err != nil {
		t.Fatalf("创建 Team: %v", err)
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

func newTeamRepositoryTestRegistry(t *testing.T, repository Repository) *Registry {
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
	registry, err := NewRegistryWithRepository(scope, repository)
	if err != nil {
		t.Fatalf("创建 Team Registry: %v", err)
	}
	return registry
}
