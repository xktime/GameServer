package managers

import (
	"context"
	"errors"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/common/session"
	"gameserver/common/utils"
	"gameserver/core/log"
	"gameserver/modules/game/internal/managers/player"
	"gameserver/modules/game/internal/managers/team"
	gameprovision "gameserver/modules/game/internal/provision"
	"gameserver/modules/game/playersession"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

const managerInitializationRetryDelay = 100 * time.Millisecond

type UserManager struct {
	*userDependencies
	actor      *userActor
	definition *actor.Definition[*userActor, string]
	mu         sync.RWMutex
	recoveryMu utils.KeyedMutex[string]
}

func (h *UserManager) currentActor() *userActor {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.actor
}

// userDependencies 由 Manager 与各代 Actor 共享；offline 仅在启动时设置。
type userDependencies struct {
	players          *player.Registry
	teams            *team.Registry
	sessions         *playersession.State
	offline          func(context.Context, int64) bool
	repository       userRepository
	intentRepository gameprovision.Repository
}

// userActor 的可变状态只允许在所属邮箱内访问。
type userActor struct {
	actor.BaseActor
	*userDependencies
	nameCache   map[string]bool
	initialized bool
}

func NewManagers(ctx context.Context, scope *actor.Scope, intentRepository gameprovision.Repository) (*UserManager, *TeamManager, error) {
	teams, err := team.NewRegistry(scope)
	if err != nil {
		return nil, nil, err
	}
	players, err := player.NewRegistry(scope, teams)
	if err != nil {
		return nil, nil, err
	}
	users, err := NewUserManager(ctx, scope, players, teams, intentRepository)
	if err != nil {
		return nil, nil, err
	}
	teamManager, err := NewTeamManager(players, teams)
	if err != nil {
		return nil, nil, err
	}
	return users, teamManager, nil
}

func NewUserManager(ctx context.Context, scope *actor.Scope, players *player.Registry, teams *team.Registry, intentRepository gameprovision.Repository) (*UserManager, error) {
	return newUserManager(ctx, scope, players, teams, mongoUserRepository{}, intentRepository)
}

func newUserManager(
	ctx context.Context,
	scope *actor.Scope,
	players *player.Registry,
	teams *team.Registry,
	repository userRepository,
	intentRepository gameprovision.Repository,
) (*UserManager, error) {
	if players == nil {
		return nil, fmt.Errorf("game: Player Registry is nil")
	}
	if teams == nil {
		return nil, fmt.Errorf("game: Team Registry is nil")
	}
	if repository == nil {
		return nil, fmt.Errorf("game: User Repository is nil")
	}
	if intentRepository == nil {
		return nil, fmt.Errorf("game: Player Provision Repository is nil")
	}
	handle := &UserManager{userDependencies: &userDependencies{
		players:          players,
		teams:            teams,
		sessions:         playersession.NewState(),
		repository:       repository,
		intentRepository: intentRepository,
	}}
	definition, err := actor.Define(scope, func(context.Context, string) (*userActor, error) {
		return &userActor{
			userDependencies: handle.userDependencies,
			nameCache:        make(map[string]bool),
		}, nil
	}, actor.DefinitionOptions{QueueCapacity: 1024})
	if err != nil {
		return nil, err
	}
	manager, err := definition.GetOrCreate(ctx, "singleton")
	if err != nil {
		return nil, err
	}
	if err := manager.initializeUntilReady(ctx); err != nil {
		return nil, err
	}
	handle.actor, handle.definition = manager, definition
	return handle, nil
}

func (m *userActor) initializeUntilReady(ctx context.Context) error {
	for {
		err := m.initialize(ctx)
		if err == nil {
			return nil
		}
		select {
		case <-time.After(managerInitializationRetryDelay):
		case <-ctx.Done():
			return errors.Join(err, ctx.Err())
		}
	}
}

func (m *userActor) initialize(ctx context.Context) error {
	_, err := actor.Command(ctx, m.Ref(), func(actor.Context) (struct{}, error) {
		if m.initialized {
			return struct{}{}, nil
		}
		if err := m.preloadNames(ctx); err != nil {
			return struct{}{}, err
		}
		m.initialized = true
		return struct{}{}, nil
	})
	return err
}

func (m *userActor) OnStop(ctx context.Context) error {
	if !m.initialized {
		return nil
	}
	now := time.Now().Unix()
	stopErrors := make([]error, 0)
	for _, active := range m.sessions.All() {
		user := active.User
		user.LastOfflineTime = now
		if err := m.repository.Save(ctx, &user); err != nil {
			stopErrors = append(stopErrors, err)
		}
	}
	return errors.Join(stopErrors...)
}

func (handle *UserManager) LoginState() *playersession.State {
	return handle.sessions
}

// SetOfflineNotifier is configured during startup, before requests are served.
func (handle *UserManager) SetOfflineNotifier(notify func(context.Context, int64) bool) {
	handle.offline = notify
}

func (handle *UserManager) UserLogin(attempt *playersession.Attempt, openID string, serverID int32, loginType message.LoginType) error {
	m := handle.currentActor()

	_, err := actor.Command(context.Background(), m.Ref(), func(execution actor.Context) (struct{}, error) {
		return struct{}{}, m.doUserLogin(execution, attempt, openID, serverID, loginType)
	})
	return err
}

// HandleProvisionFailure 关闭与失败的首次登录创建意图关联的
// 在线会话，同时保留已持久化的创建意图，
// 以便后续重试。
func (handle *UserManager) HandleProvisionFailure(ctx context.Context, intent gameprovision.Intent) error {
	m := handle.currentActor()

	if ctx == nil {
		return errors.New("game: provision failure context is nil")
	}
	_, err := actor.Command(ctx, m.Ref(), func(execution actor.Context) (struct{}, error) {
		active, ok := m.sessions.Current(intent.AccountID)
		if !ok {
			return struct{}{}, nil
		}
		active, ok = m.sessions.Close(active.Snapshot)
		if !ok {
			return struct{}{}, nil
		}
		return struct{}{}, m.doUserOffline(execution, active)
	})
	return err
}

func (m *userActor) doUserLogin(ctx context.Context, attempt *playersession.Attempt, openID string, serverID int32, loginType message.LoginType) error {
	if openID == "" {
		return fmt.Errorf("game: invalid login identity for server %d", serverID)
	}
	accountID := fmt.Sprintf("%d_%s", serverID, openID)
	if !attempt.IsCurrent(accountID) {
		return playersession.ErrStaleAttempt
	}
	existing, online := m.sessions.Current(accountID)

	// Login metadata can also be newer than Mongo, especially while a first
	// login's durable Intent is still being materialized.
	user := &existing.User
	if !online {
		var err error
		user, err = m.repository.FindByLogin(ctx, openID, serverID)
		if err != nil {
			return err
		}
	}
	isNew := user == nil
	loginTime := time.Now().Unix()
	var playerData gameprovision.PlayerData
	var teamData gameprovision.TeamData
	if isNew {
		user = &models.User{
			AccountId:  accountID,
			OpenId:     openID,
			ServerId:   serverID,
			PlayerId:   utils.FlakeId(),
			Platform:   loginType,
			CreateTime: loginTime,
		}
		intent, findErr := m.intentRepository.FindByAccount(ctx, accountID)
		if findErr != nil {
			return findErr
		}
		if intent != nil {
			// 重试首次登录时，以待处理或失败的持久化创建意图为准，
			// 复用其中的标识，避免重复创建
			// 玩家或队伍。
			*user = intent.User
			user.RecordLogin(loginTime)
			playerData = intent.Player
			intent.User = *user
			intent.Status = gameprovision.StatusPending
			intent.Attempts = 0
			intent.NextAttemptAt = time.Time{}
			intent.LastError = ""
			intent.UpdatedAt = time.Now()
			if err := m.intentRepository.Save(ctx, intent); err != nil {
				return err
			}
		} else {
			teamID := utils.FlakeId()
			user.RecordLogin(loginTime)
			playerData, teamData = gameprovision.NewInitialData(*user, teamID)
			intent = &gameprovision.Intent{
				AccountID:     accountID,
				IntentID:      uuid.NewString(),
				Version:       gameprovision.CurrentVersion,
				Status:        gameprovision.StatusPending,
				NextAttemptAt: time.Now(),
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
				User:          *user,
				Player:        playerData,
				Team:          teamData,
			}
			if err := m.intentRepository.Save(ctx, intent); err != nil {
				return err
			}
		}
	} else {
		user.RecordLogin(loginTime)
	}

	if !attempt.IsCurrent(accountID) {
		return playersession.ErrStaleAttempt
	}
	var initial *gameprovision.PlayerData
	if isNew {
		initial = &playerData
	}
	prepared, err := m.players.PrepareLogin(ctx, *user, initial, online)
	if err != nil {
		return err
	}
	response := &message.S2C_Login{
		LoginResult: 1,
		LoginInfo: &message.LoginInfo{
			OpenId:        user.OpenId,
			LastLoginTime: int32(user.LastOfflineTime),
			TotalDays:     user.TotalLoginDays,
			IsAccept:      false,
		},
	}
	committed, err := prepared.Commit(ctx, attempt, *user, response)
	if !committed {
		cleanupErr := prepared.Abort(context.Background())
		if err != nil || cleanupErr != nil {
			return errors.Join(err, cleanupErr)
		}
		return playersession.ErrStaleAttempt
	}
	// Player 文档可能仍在等待创建意图落盘，先登记已提交的初始名字。
	if isNew && playerData.PlayerInfo != nil {
		m.addNameToCache(playerData.PlayerInfo.PlayerName)
	}
	if err := m.repository.Save(ctx, user); err != nil {
		log.Error("保存登录用户 %s 失败: %v", accountID, err)
	}
	return err
}

func (handle *UserManager) ClosePlayerSession(snapshot session.PlayerSession) {
	m := handle.currentActor()

	if err := actor.Tell(context.Background(), m.Ref(), func(execution actor.Context) error {
		active, ok := m.sessions.Close(snapshot)
		if !ok {
			return nil
		}
		return m.doUserOffline(execution, active)
	}); err != nil {
		log.Error("提交 Player Session %d 下线失败: %v", snapshot.ID, err)
	}
}

func (m *userActor) doUserOffline(ctx context.Context, active playersession.Active) error {
	user := active.User
	offlineErrors := make([]error, 0)
	// Only a verified current Session reaches this point. Room no longer treats
	// raw transport CloseAgent events as Player offline events.
	if m.offline != nil {
		m.offline(ctx, user.PlayerId)
	}
	user.LastOfflineTime = time.Now().Unix()
	if err := m.repository.Save(ctx, &user); err != nil {
		offlineErrors = append(offlineErrors, err)
	}

	playerSnapshot, err := m.players.Offline(ctx, user.PlayerId)
	if err != nil && !errors.Is(err, actor.ErrActorNotFound) {
		offlineErrors = append(offlineErrors, err)
	}
	if playerSnapshot.TeamID != 0 {
		if leaveErr := m.teams.Leave(ctx, playerSnapshot.TeamID, user.PlayerId); leaveErr != nil &&
			!errors.Is(leaveErr, actor.ErrActorNotFound) {
			offlineErrors = append(offlineErrors, leaveErr)
		}
	}
	if active.Agent != nil {
		active.Agent.Close()
	}
	log.Debug("User offline: %s, PlayerId: %d", user.AccountId, user.PlayerId)
	return errors.Join(offlineErrors...)
}

func (handle *UserManager) ModifyName(playerID int64, name string) (message.Result, string) {
	m := handle.currentActor()

	type result struct {
		status message.Result
		name   string
	}
	response, err := actor.Command(context.Background(), m.Ref(), func(execution actor.Context) (result, error) {
		status, appliedName, err := m.players.ModifyName(execution, playerID, name)
		if err != nil {
			return result{}, err
		}
		if status == message.Result_Success {
			m.addNameToCache(appliedName)
		}
		return result{status: status, name: appliedName}, nil
	})
	if err != nil {
		log.Error("修改玩家 %d 名称失败: %v", playerID, err)
		return message.Result_Fail, ""
	}
	return response.status, response.name
}

func (handle *UserManager) CheckName(playerName string) message.Result {
	m := handle.currentActor()

	response, err := actor.Query(context.Background(), m.Ref(), func(actor.Context) (message.Result, error) {
		return m.checkName(playerName), nil
	})
	if err != nil {
		log.Error("检查名字失败: %v", err)
		return message.Result_Fail
	}
	if response != message.Result_Success {
		return response
	}
	// 缓存未命中时的数据库读取不占用会话邮箱。
	exists, err := handle.players.NameExists(context.Background(), playerName)
	if err != nil {
		log.Error("CheckName query database failed: %v", err)
		return message.Result_Fail
	}
	response, err = actor.Command(context.Background(), m.Ref(), func(actor.Context) (message.Result, error) {
		if exists {
			m.addNameToCache(playerName)
		}
		// 数据库读取期间，登录或改名可能已经登记了这个名字。
		return m.checkName(playerName), nil
	})
	if err != nil {
		log.Error("检查名字失败: %v", err)
		return message.Result_Fail
	}
	return response
}

func (m *userActor) checkName(playerName string) message.Result {
	if len(playerName) < 2 || len(playerName) > 20 {
		return message.Result_Illegal
	}
	if m.nameCache[playerName] {
		return message.Result_Duplicate
	}
	return message.Result_Success
}

func (handle *UserManager) ModifyAvatarSuffix(playerID int64, avatar string) (message.Result, string) {
	status, appliedAvatar, err := handle.players.ModifyAvatarSuffix(context.Background(), playerID, avatar)
	if err != nil {
		log.Error("修改玩家 %d 头像失败: %v", playerID, err)
		return message.Result_Fail, ""
	}
	return status, appliedAvatar
}

func (handle *UserManager) GetPlayerSnapshot(ctx context.Context, playerID int64) (player.Snapshot, bool, error) {
	snapshot, err := handle.players.Snapshot(ctx, playerID)
	if errors.Is(err, actor.ErrActorNotFound) {
		return player.Snapshot{}, false, nil
	}
	return snapshot, err == nil, err
}

func (handle *UserManager) GetPlayerInfo(playerID int64) (*message.PlayerInfo, bool) {
	snapshot, err := handle.players.Snapshot(context.Background(), playerID)
	if err == nil && snapshot.PlayerInfo != nil {
		return snapshot.PlayerInfo.ToMsg(), true
	}
	if !errors.Is(err, actor.ErrActorNotFound) {
		return nil, false
	}
	offlinePlayer, err := handle.players.StoredSnapshot(context.Background(), playerID)
	if err != nil {
		if !errors.Is(err, actor.ErrActorNotFound) {
			log.Error("获取离线玩家 %d 失败: %v", playerID, err)
		}
		return nil, false
	}
	if offlinePlayer.PlayerInfo == nil {
		return nil, false
	}
	return offlinePlayer.PlayerInfo.ToMsg(), true
}

func (handle *UserManager) SendToPlayer(playerID int64, msg proto.Message) {
	if err := handle.players.SendToClient(context.Background(), playerID, msg); err != nil && !errors.Is(err, actor.ErrActorNotFound) {
		log.Error("发送消息给玩家 %d 失败: %v", playerID, err)
	}
}

func (m *userActor) preloadNames(ctx context.Context) error {
	names, err := m.players.StoredNames(ctx)
	if err != nil {
		return fmt.Errorf("preload player names: %w", err)
	}
	for _, name := range names {
		m.addNameToCache(name)
	}
	log.Debug("Preloaded %d names from database", len(m.nameCache))
	return nil
}

func (m *userActor) addNameToCache(playerName string) {
	m.nameCache[playerName] = true
}
