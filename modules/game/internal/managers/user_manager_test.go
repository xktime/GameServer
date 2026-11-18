package managers

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/common/session"
	"gameserver/common/utils"
	"gameserver/core/gate"
	"gameserver/modules/game/internal/managers/player"
	"gameserver/modules/game/internal/managers/team"
	playermodel "gameserver/modules/game/internal/models/player"
	gameprovision "gameserver/modules/game/internal/provision"
	"gameserver/modules/game/playersession"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeUserRepository struct {
	findErr   error
	findCalls []struct {
		openID   string
		serverID int32
	}
	saveErr error
	saved   []models.User
}

type stubTeamRepository struct{}

func (stubTeamRepository) FindByID(context.Context, int64) (*team.Team, error) {
	return nil, nil
}

func (stubTeamRepository) Save(context.Context, *team.Team) error  { return nil }
func (stubTeamRepository) DeleteByID(context.Context, int64) error { return nil }

func (r *fakeUserRepository) FindByLogin(ctx context.Context, openID string, serverID int32) (*models.User, error) {
	r.findCalls = append(r.findCalls, struct {
		openID   string
		serverID int32
	}{openID: openID, serverID: serverID})
	return nil, r.findErr
}

func (r *fakeUserRepository) Save(ctx context.Context, user *models.User) error {
	r.saved = append(r.saved, *user)
	return r.saveErr
}

type userManagerAgent struct {
	mu          sync.Mutex
	id          gate.ConnectionID
	closed      bool
	userData    any
	writes      []any
	sequences   []uint32
	beforeWrite func()
}

var _ gate.Agent = (*userManagerAgent)(nil)

func (a *userManagerAgent) WriteMsg(msg any) { a.WriteMsgWithSeq(msg, 0) }
func (a *userManagerAgent) WriteMsgWithSeq(msg any, sequence uint32) {
	if a.beforeWrite != nil {
		a.beforeWrite()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.writes = append(a.writes, msg)
	a.sequences = append(a.sequences, sequence)
}
func (*userManagerAgent) LocalAddr() net.Addr               { return nil }
func (*userManagerAgent) RemoteAddr() net.Addr              { return nil }
func (a *userManagerAgent) ConnectionID() gate.ConnectionID { return a.id }
func (a *userManagerAgent) Close()                          { a.mu.Lock(); defer a.mu.Unlock(); a.closed = true }
func (*userManagerAgent) Destroy()                          {}
func (a *userManagerAgent) UserData() any                   { a.mu.Lock(); defer a.mu.Unlock(); return a.userData }
func (a *userManagerAgent) SetUserData(data any)            { a.mu.Lock(); defer a.mu.Unlock(); a.userData = data }

var userTestConnectionID atomic.Uint64

func beginUserLogin(t *testing.T, manager *UserManager, agent *userManagerAgent, sequence uint32) *playersession.Attempt {
	t.Helper()
	if agent.id == 0 {
		agent.id = gate.ConnectionID(userTestConnectionID.Add(1))
	}
	manager.actor.sessions.Register(agent)
	attempt, err := manager.actor.sessions.Begin(agent.id, sequence)
	if err != nil {
		t.Fatal(err)
	}
	return attempt
}

func testUserLogin(t *testing.T, manager *UserManager, agent *userManagerAgent, openID string, serverID int32, loginType message.LoginType) *message.S2C_Login {
	t.Helper()
	attempt := beginUserLogin(t, manager, agent, 1)
	if err := manager.UserLogin(attempt, openID, serverID, loginType); err != nil {
		attempt.Fail()
	}
	agent.mu.Lock()
	defer agent.mu.Unlock()
	for _, msg := range agent.writes {
		if response, ok := msg.(*message.S2C_Login); ok {
			return response
		}
	}
	t.Fatal("missing login reply")
	return nil
}

func TestUserManagerInitializationCommandCanRetry(t *testing.T) {
	system := actor.NewActorSystem(time.Second)
	t.Cleanup(func() {
		if err := system.Stop(context.Background()); err != nil {
			t.Errorf("停止 ActorSystem: %v", err)
		}
	})
	scope, err := system.NewScope("user-manager-initialization-test")
	if err != nil {
		t.Fatalf("创建 Scope: %v", err)
	}
	teams, err := team.NewRegistryWithRepository(scope, stubTeamRepository{})
	if err != nil {
		t.Fatalf("创建 Team Registry: %v", err)
	}
	loadFailure := errors.New("database unavailable")
	attempts := 0
	players, err := player.NewRegistryWithRepository(scope, teams, &userPlayerRepository{load: func(context.Context) ([]player.Player, error) {
		attempts++
		if attempts == 1 {
			return nil, loadFailure
		}
		return []player.Player{{PlayerInfo: &playermodel.PlayerInfo{PlayerName: "重试成功"}}}, nil
	}})
	if err != nil {
		t.Fatalf("创建 Player Registry: %v", err)
	}
	manager, err := newUserManager(context.Background(), scope, players, teams, &fakeUserRepository{}, gameprovision.NewMemoryRepository())
	if err != nil {
		t.Fatalf("初始化 UserManager: %v", err)
	}

	if !manager.actor.initialized || !manager.actor.nameCache["重试成功"] || attempts != 2 {
		t.Fatalf("重试结果不正确: initialized=%t names=%v attempts=%d", manager.actor.initialized, manager.actor.nameCache, attempts)
	}
}

func TestUserManagerLoginCreatesNewUserThroughRepository(t *testing.T) {
	saveFailure := errors.New("save unavailable")
	repository := &fakeUserRepository{saveErr: saveFailure}
	manager := newUserManagerRepositoryTestHarness(t, repository, gameprovision.NewMemoryRepository())
	agent := &userManagerAgent{}

	response := testUserLogin(t, manager, agent, "new-open-id", 7, message.LoginType_None)
	if response.LoginResult != 1 {
		t.Fatalf("新账号登录结果 = %d，期望成功", response.LoginResult)
	}
	if len(repository.findCalls) != 1 || repository.findCalls[0].openID != "new-open-id" || repository.findCalls[0].serverID != 7 {
		t.Fatalf("Repository 查询 = %#v，期望 new-open-id/7", repository.findCalls)
	}
	if len(repository.saved) != 1 {
		t.Fatalf("Repository 保存次数 = %d，期望 1", len(repository.saved))
	}
	saved := repository.saved[0]
	if saved.AccountId != "7_new-open-id" || saved.OpenId != "new-open-id" || saved.ServerId != 7 || saved.PlayerId == 0 {
		t.Fatalf("新建 User = %#v", saved)
	}
	snapshot, ok := session.FromData(agent.UserData())
	if !ok || snapshot.AccountID != saved.AccountId || snapshot.PlayerID != saved.PlayerId {
		t.Fatalf("登录后的 Player Session = %#v, %t", snapshot, ok)
	}

	// 首次保存失败不应反转已经成功建立的登录；清除错误以便测试清理时正常持久化。
	repository.saveErr = nil
}

func TestUserManagerFirstLoginReservesNameBeforeProvisioning(t *testing.T) {
	for _, replay := range []bool{false, true} {
		name := "new-intent"
		if replay {
			name = "pending-intent"
		}
		t.Run(name, func(t *testing.T) {
			intents := gameprovision.NewMemoryRepository()
			manager := newUserManagerRepositoryTestHarness(t, &fakeUserRepository{}, intents)
			playerName := "initial-name"
			if replay {
				user := models.User{AccountId: "7_initial-name", OpenId: "initial-name", ServerId: 7, PlayerId: 42}
				playerData, teamData := gameprovision.NewInitialData(user, 8)
				playerName = "pending-name"
				playerData.PlayerInfo.PlayerName = playerName
				if err := intents.Save(context.Background(), &gameprovision.Intent{
					AccountID: user.AccountId, User: user, Player: playerData, Team: teamData,
				}); err != nil {
					t.Fatal(err)
				}
			}
			if result := manager.CheckName(playerName); result != message.Result_Success {
				t.Fatalf("name %q before login = %v; want Success", playerName, result)
			}
			if response := testUserLogin(t, manager, &userManagerAgent{}, "initial-name", 7, message.LoginType_None); response.LoginResult != 1 {
				t.Fatalf("first login = %#v", response)
			}
			if result := manager.CheckName(playerName); result != message.Result_Duplicate {
				t.Fatalf("committed name %q before provisioning = %v; want Duplicate", playerName, result)
			}
		})
	}
}

func TestUserManagerLoginFailsWhenRepositoryLoadFails(t *testing.T) {
	loadFailure := errors.New("load unavailable")
	repository := &fakeUserRepository{findErr: loadFailure}
	manager := newUserManagerRepositoryTestHarness(t, repository, gameprovision.NewMemoryRepository())
	agent := &userManagerAgent{}

	response := testUserLogin(t, manager, agent, "existing-open-id", 8, message.LoginType_None)
	if response.LoginResult != -1 {
		t.Fatalf("查询失败时登录结果 = %d，期望失败", response.LoginResult)
	}
	if len(repository.saved) != 0 {
		t.Fatalf("查询失败后仍保存了 User: %#v", repository.saved)
	}
	if agent.UserData() != nil {
		t.Fatalf("查询失败后建立了 Session: %#v", agent.UserData())
	}
}

func TestUserManagerLoginReusesPendingProvisionIntent(t *testing.T) {
	repository := &fakeUserRepository{}
	intents := gameprovision.NewMemoryRepository()
	manager := newUserManagerRepositoryTestHarness(t, repository, intents)

	first := &userManagerAgent{}
	if response := testUserLogin(t, manager, first, "pending-open-id", 9, message.LoginType_None); response.LoginResult != 1 {
		t.Fatalf("首次登录结果 = %d，期望成功", response.LoginResult)
	}
	firstIntent, err := intents.FindByAccount(context.Background(), "9_pending-open-id")
	if err != nil || firstIntent == nil {
		t.Fatalf("首次登录未创建 Intent: %v, %#v", err, firstIntent)
	}

	second := &userManagerAgent{}
	if response := testUserLogin(t, manager, second, "pending-open-id", 9, message.LoginType_None); response.LoginResult != 1 {
		t.Fatalf("重试登录结果 = %d，期望成功", response.LoginResult)
	}
	secondIntent, err := intents.FindByAccount(context.Background(), "9_pending-open-id")
	if err != nil || secondIntent == nil {
		t.Fatalf("重试登录读取 Intent 失败: %v, %#v", err, secondIntent)
	}
	if secondIntent.IntentID != firstIntent.IntentID ||
		secondIntent.Player.PlayerID != firstIntent.Player.PlayerID ||
		secondIntent.Team.TeamID != firstIntent.Team.TeamID {
		t.Fatalf("重试登录创建了新的身份: first=%#v second=%#v", firstIntent, secondIntent)
	}
}

func TestUserManagerTerminatesSessionAfterProvisionFailure(t *testing.T) {
	manager := newUserManagerRepositoryTestHarness(t, &fakeUserRepository{}, gameprovision.NewMemoryRepository())
	agent := &userManagerAgent{}
	if response := testUserLogin(t, manager, agent, "failed-open-id", 10, message.LoginType_None); response.LoginResult != 1 {
		t.Fatalf("登录结果 = %d，期望成功", response.LoginResult)
	}

	if err := manager.HandleProvisionFailure(context.Background(), gameprovision.Intent{
		AccountID: "10_failed-open-id",
	}); err != nil {
		t.Fatalf("处理终态失败 Intent: %v", err)
	}
	if !agent.closed {
		t.Fatal("终态失败后未断开登录会话")
	}
	if agent.UserData() != nil {
		t.Fatalf("终态失败后仍保留会话数据: %#v", agent.UserData())
	}
}

func newUserManagerRepositoryTestHarness(t *testing.T, repository userRepository, intentRepository gameprovision.Repository) *UserManager {
	t.Helper()
	utils.InitSnowflake(1)
	system := actor.NewActorSystem(time.Second)
	t.Cleanup(func() {
		if err := system.Stop(context.Background()); err != nil {
			t.Errorf("停止 ActorSystem: %v", err)
		}
	})
	scope, err := system.NewScope("user-manager-repository-test")
	if err != nil {
		t.Fatalf("创建 Scope: %v", err)
	}
	teams, err := team.NewRegistryWithRepository(scope, stubTeamRepository{})
	if err != nil {
		t.Fatalf("创建 Team Registry: %v", err)
	}
	players, err := player.NewRegistryWithRepository(scope, teams, &userPlayerRepository{})
	if err != nil {
		t.Fatalf("创建 Player Registry: %v", err)
	}
	manager, err := newUserManager(context.Background(), scope, players, teams, repository, intentRepository)
	if err != nil {
		t.Fatalf("创建 UserManager: %v", err)
	}
	return manager
}
