package player

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	"gameserver/modules/game/internal/managers/team"
	playermodel "gameserver/modules/game/internal/models/player"
	gameprovision "gameserver/modules/game/internal/provision"
	"gameserver/modules/game/playersession"
	"testing"
	"time"
)

type fakePlayerRepository struct {
	loaded  *Player
	loadErr error
	saveErr error
	loadIDs []int64
	saved   []Snapshot
}

func (r *fakePlayerRepository) FindByID(playerID int64) (*Player, error) {
	r.loadIDs = append(r.loadIDs, playerID)
	if r.loaded == nil {
		return nil, r.loadErr
	}
	copyOfPlayer := *r.loaded
	if r.loaded.PlayerInfo != nil {
		copyOfInfo := *r.loaded.PlayerInfo
		copyOfPlayer.PlayerInfo = &copyOfInfo
	}
	return &copyOfPlayer, r.loadErr
}

func (r *fakePlayerRepository) Save(current *Player) error {
	var info *playermodel.PlayerInfo
	if current.PlayerInfo != nil {
		copyOfInfo := *current.PlayerInfo
		info = &copyOfInfo
	}
	r.saved = append(r.saved, Snapshot{
		PlayerID:   current.PlayerId,
		TeamID:     current.TeamId,
		TowerLevel: current.TowerLevel,
		PlayerInfo: info,
	})
	return r.saveErr
}

func TestOfflinePreservesFinalSnapshotWhenSaveFails(t *testing.T) {
	saveFailure := errors.New("save unavailable")
	repository := &fakePlayerRepository{loaded: &Player{PlayerId: 42}}
	registry := newPlayerRepositoryTestRegistry(t, repository)
	t.Cleanup(func() { repository.saveErr = nil })
	user := models.User{AccountId: "7_offline", PlayerId: 42}
	prepared, err := registry.PrepareLogin(context.Background(), user, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	commitTestPlayer(t, prepared, &lifecycleAgent{}, user)
	before, err := registry.Snapshot(context.Background(), user.PlayerId)
	if err != nil || before.TeamID == 0 {
		t.Fatalf("online snapshot = %#v, %v", before, err)
	}
	repository.saveErr = saveFailure
	after, err := registry.Offline(context.Background(), user.PlayerId)
	if !errors.Is(err, saveFailure) || after.PlayerID != before.PlayerID || after.TeamID != before.TeamID {
		t.Fatalf("offline snapshot = %#v, error = %v", after, err)
	}
}

func TestRegistryLoginLoadsAndSavesExistingPlayerThroughRepository(t *testing.T) {
	repository := &fakePlayerRepository{loaded: &Player{
		PlayerId:   42,
		PlayerInfo: &playermodel.PlayerInfo{PlayerId: 42, ServerId: 7, PlayerName: "existing-player", Level: 10},
		TowerLevel: 9,
	}}
	registry := newPlayerRepositoryTestRegistry(t, repository)
	agent := &lifecycleAgent{}

	user := models.User{AccountId: "7_existing", PlayerId: 42, ServerId: 7}
	prepared, err := registry.PrepareLogin(context.Background(), user, nil, false)
	if err != nil {
		t.Fatalf("老玩家登录: %v", err)
	}
	commitTestPlayer(t, prepared, agent, user)
	if len(repository.loadIDs) != 1 || repository.loadIDs[0] != 42 {
		t.Fatalf("Repository 加载 ID = %v，期望 [42]", repository.loadIDs)
	}
	snapshot, err := registry.Snapshot(context.Background(), 42)
	if err != nil {
		t.Fatalf("读取老玩家快照: %v", err)
	}
	if snapshot.PlayerInfo == nil || snapshot.PlayerInfo.PlayerName != "existing-player" || snapshot.TowerLevel != 9 {
		t.Fatalf("老玩家快照 = %#v", snapshot)
	}

	if err := registry.Stop(context.Background(), 42); err != nil {
		t.Fatalf("停止老玩家: %v", err)
	}
	if len(repository.saved) != 1 || repository.saved[0].PlayerID != 42 || repository.saved[0].TowerLevel != 9 {
		t.Fatalf("Repository 保存 = %#v", repository.saved)
	}
}

func TestRegistryLoginNewPlayerSkipsRepositoryLoad(t *testing.T) {
	repository := &fakePlayerRepository{loadErr: errors.New("load should not run")}
	registry := newPlayerRepositoryTestRegistry(t, repository)
	agent := &lifecycleAgent{}

	user := models.User{AccountId: "8_new-player", PlayerId: 43, OpenId: "new-player", ServerId: 8}
	prepared, err := registry.PrepareLogin(context.Background(), user, &gameprovision.PlayerData{
		PlayerID: 43,
		TeamID:   utils.FlakeId(),
		PlayerInfo: &playermodel.PlayerInfo{
			PlayerId:   43,
			ServerId:   8,
			PlayerName: "new-player",
			Level:      1,
		},
	}, false)
	if err != nil {
		t.Fatalf("新玩家登录: %v", err)
	}
	commitTestPlayer(t, prepared, agent, user)
	if len(repository.loadIDs) != 0 {
		t.Fatalf("新玩家触发了 Repository 加载: %v", repository.loadIDs)
	}
	if err := registry.Stop(context.Background(), 43); err != nil {
		t.Fatalf("停止新玩家: %v", err)
	}
	if len(repository.saved) != 1 || repository.saved[0].PlayerInfo == nil || repository.saved[0].PlayerInfo.PlayerName != "new-player" {
		t.Fatalf("新玩家保存 = %#v", repository.saved)
	}
}

func TestRegistryLoginPropagatesExistingPlayerLoadFailure(t *testing.T) {
	loadFailure := errors.New("load unavailable")
	repository := &fakePlayerRepository{loadErr: loadFailure}
	registry := newPlayerRepositoryTestRegistry(t, repository)

	_, err := registry.PrepareLogin(context.Background(), models.User{PlayerId: 44}, nil, false)
	if !errors.Is(err, loadFailure) {
		t.Fatalf("老玩家加载错误 = %v，期望 %v", err, loadFailure)
	}
	if len(repository.saved) != 0 {
		t.Fatalf("加载失败后仍保存了 Player: %#v", repository.saved)
	}
}

func commitTestPlayer(t *testing.T, prepared *PreparedLogin, agent *lifecycleAgent, user models.User) {
	t.Helper()
	state := playersession.NewState()
	state.Register(agent)
	attempt, err := state.Begin(agent.ConnectionID(), 1)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := prepared.Commit(context.Background(), attempt, user, &message.S2C_Login{LoginResult: 1})
	if err != nil || !committed {
		t.Fatalf("commit = %t, %v", committed, err)
	}
}

func newPlayerRepositoryTestRegistry(t *testing.T, repository playerRepository) *Registry {
	t.Helper()
	utils.InitSnowflake(1)
	system := actor.NewActorSystem(time.Second)
	t.Cleanup(func() {
		if err := system.Stop(context.Background()); err != nil {
			t.Errorf("停止 ActorSystem: %v", err)
		}
	})
	scope, err := system.NewScope("player-repository-test")
	if err != nil {
		t.Fatalf("创建 Scope: %v", err)
	}
	teams, err := team.NewRegistry(scope)
	if err != nil {
		t.Fatalf("创建 Team Registry: %v", err)
	}
	registry, err := newRegistry(scope, teams, repository)
	if err != nil {
		t.Fatalf("创建 Player Registry: %v", err)
	}
	return registry
}
