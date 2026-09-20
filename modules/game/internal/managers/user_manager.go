package managers

import (
	"context"
	"errors"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/db/mongodb"
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/common/session"
	"gameserver/common/utils"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game/internal/managers/player"
	provisionmanager "gameserver/modules/game/internal/managers/provision"
	"gameserver/modules/game/internal/managers/team"
	gameprovision "gameserver/modules/game/internal/provision"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"google.golang.org/protobuf/proto"
)

const managerInitializationRetryDelay = 100 * time.Millisecond

type UserManager struct {
	actor.BaseActor
	players          *player.Registry
	teams            *team.Registry
	sessions         *playerSessions
	nameCache        map[string]bool
	nameBloomFilter  *utils.BloomFilter
	repository       userRepository                  `bson:"-"`
	intentRepository provisionmanager.Repository     `bson:"-"`
	loadPlayers      func() ([]player.Player, error) `bson:"-"`
	initialized      bool                            `bson:"-"`
}

func NewManagers(ctx context.Context, scope *actor.Scope, intentRepository provisionmanager.Repository) (*UserManager, *TeamManager, error) {
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
	teamManager, err := NewTeamManager(ctx, scope, players, teams)
	if err != nil {
		return nil, nil, err
	}
	return users, teamManager, nil
}

func NewUserManager(ctx context.Context, scope *actor.Scope, players *player.Registry, teams *team.Registry, intentRepository provisionmanager.Repository) (*UserManager, error) {
	return newUserManager(ctx, scope, players, teams, mongoUserRepository{}, func() ([]player.Player, error) {
		return mongodb.FindAll[player.Player](bson.M{})
	}, intentRepository)
}

func newUserManager(
	ctx context.Context,
	scope *actor.Scope,
	players *player.Registry,
	teams *team.Registry,
	repository userRepository,
	loadPlayers func() ([]player.Player, error),
	intentRepository provisionmanager.Repository,
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
	if loadPlayers == nil {
		return nil, fmt.Errorf("game: Player loader is nil")
	}
	if intentRepository == nil {
		return nil, fmt.Errorf("game: Player Provision Repository is nil")
	}
	definition, err := actor.Define(scope, actor.User, func(context.Context, string) (*UserManager, error) {
		manager := &UserManager{
			players:          players,
			teams:            teams,
			sessions:         newPlayerSessions(),
			nameCache:        make(map[string]bool),
			nameBloomFilter:  utils.NewBloomFilter(1000000, 7),
			repository:       repository,
			intentRepository: intentRepository,
			loadPlayers:      loadPlayers,
		}
		return manager, nil
	})
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
	return manager, nil
}

func (m *UserManager) initializeUntilReady(ctx context.Context) error {
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

func (m *UserManager) initialize(ctx context.Context) error {
	_, err := actor.Call(ctx, m.Ref(), func(actor.Context) (struct{}, error) {
		if m.initialized {
			return struct{}{}, nil
		}
		if err := m.preloadNames(); err != nil {
			return struct{}{}, err
		}
		m.initialized = true
		return struct{}{}, nil
	})
	return err
}

func (m *UserManager) OnStop(context.Context) error {
	if !m.initialized {
		return nil
	}
	now := time.Now().Unix()
	stopErrors := make([]error, 0)
	for _, active := range m.sessions.all() {
		user := active.user
		user.LastOfflineTime = now
		if err := m.repository.Save(user); err != nil {
			stopErrors = append(stopErrors, err)
		}
	}
	return errors.Join(stopErrors...)
}

func (m *UserManager) UserLogin(agent gate.Agent, openID string, serverID int32, loginType message.LoginType) *message.S2C_Login {
	response, err := actor.Call(context.Background(), m.Ref(), func(execution actor.Context) (*message.S2C_Login, error) {
		return m.doUserLogin(execution, agent, openID, serverID, loginType)
	})
	if err != nil {
		log.Error("用户登录失败: %v", err)
		return &message.S2C_Login{LoginResult: -1}
	}
	return response
}

// HandleProvisionFailure 关闭与失败的首次登录创建意图关联的
// 在线会话，同时保留已持久化的创建意图，
// 以便后续重试。
func (m *UserManager) HandleProvisionFailure(ctx context.Context, intent gameprovision.Intent) error {
	if ctx == nil {
		return errors.New("game: provision failure context is nil")
	}
	_, err := actor.Call(ctx, m.Ref(), func(execution actor.Context) (struct{}, error) {
		active, ok := m.sessions.current(intent.AccountID)
		if !ok {
			return struct{}{}, nil
		}
		active, ok = m.sessions.close(active.snapshot)
		if !ok {
			return struct{}{}, nil
		}
		return struct{}{}, m.doUserOffline(execution, active, true)
	})
	return err
}

func (m *UserManager) doUserLogin(ctx context.Context, agent gate.Agent, openID string, serverID int32, loginType message.LoginType) (*message.S2C_Login, error) {
	if agent == nil {
		return &message.S2C_Login{LoginResult: -1}, nil
	}
	if _, authenticated := session.FromData(agent.UserData()); authenticated {
		return &message.S2C_Login{LoginResult: -1}, nil
	}
	accountID := fmt.Sprintf("%d_%s", serverID, openID)
	if existingSession, exists := m.sessions.current(accountID); exists {
		log.Debug("UserLogin: user already online (顶号操作): %s", accountID)
		closedSession, _ := m.sessions.close(existingSession.snapshot)
		if err := m.doUserOffline(ctx, closedSession, true); err != nil {
			return nil, err
		}
	}

	user, err := m.repository.FindByLogin(openID, serverID)
	if err != nil {
		return nil, err
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
		intent, findErr := m.intentRepository.FindByAccount(accountID)
		if findErr != nil {
			return nil, findErr
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
			if err := m.intentRepository.Save(intent); err != nil {
				return nil, err
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
			if err := m.intentRepository.Save(intent); err != nil {
				return nil, err
			}
		}
	} else {
		user.RecordLogin(loginTime)
	}

	var loginErr error
	if isNew {
		loginErr = m.players.LoginProvision(ctx, agent, *user, playerData)
	} else {
		loginErr = m.players.Login(ctx, agent, *user)
	}
	if loginErr != nil {
		return nil, loginErr
	}
	activeSession, err := m.sessions.open(user, agent)
	if err != nil {
		_ = m.players.Stop(ctx, user.PlayerId)
		return nil, err
	}
	agent.SetUserData(activeSession.snapshot)
	if err := m.repository.Save(user); err != nil {
		log.Error("保存登录用户 %s 失败: %v", accountID, err)
	}
	return &message.S2C_Login{
		LoginResult: 1,
		LoginInfo: &message.LoginInfo{
			OpenId:        user.OpenId,
			LastLoginTime: int32(user.LastOfflineTime),
			TotalDays:     user.TotalLoginDays,
			IsAccept:      false,
		},
	}, nil
}

func (m *UserManager) ClosePlayerSession(snapshot session.PlayerSession) {
	if err := actor.Tell(context.Background(), m.Ref(), func(execution actor.Context) error {
		active, ok := m.sessions.close(snapshot)
		if !ok {
			return nil
		}
		return m.doUserOffline(execution, active, true)
	}); err != nil {
		log.Error("提交 Player Session %d 下线失败: %v", snapshot.ID, err)
	}
}

func (m *UserManager) doUserOffline(ctx context.Context, active activePlayerSession, save bool) error {
	user := active.user
	offlineErrors := make([]error, 0)
	if save {
		user.LastOfflineTime = time.Now().Unix()
		if err := m.repository.Save(user); err != nil {
			offlineErrors = append(offlineErrors, err)
		}
	}

	playerSnapshot, err := m.players.Snapshot(ctx, user.PlayerId)
	if err == nil {
		if sendErr := m.players.SendToClient(ctx, user.PlayerId, &message.S2C_Logout{}); sendErr != nil {
			offlineErrors = append(offlineErrors, sendErr)
		}
		if stopErr := m.players.Stop(ctx, user.PlayerId); stopErr != nil {
			offlineErrors = append(offlineErrors, stopErr)
		}
		if playerSnapshot.TeamID != 0 {
			if leaveErr := m.teams.Leave(ctx, playerSnapshot.TeamID, user.PlayerId); leaveErr != nil &&
				!errors.Is(leaveErr, actor.ErrActorStopped) {
				offlineErrors = append(offlineErrors, leaveErr)
			}
		}
	} else if !errors.Is(err, actor.ErrActorStopped) {
		offlineErrors = append(offlineErrors, err)
	}
	if active.agent != nil {
		active.agent.SetUserData(nil)
		active.agent.Close()
	}
	log.Debug("User offline: %s, PlayerId: %d", user.AccountId, user.PlayerId)
	return errors.Join(offlineErrors...)
}

func (m *UserManager) ModifyName(playerID int64, name string) (message.Result, string) {
	type result struct {
		status message.Result
		name   string
	}
	response, err := actor.Call(context.Background(), m.Ref(), func(execution actor.Context) (result, error) {
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

func (m *UserManager) CheckName(playerName string) message.Result {
	response, err := actor.Call(context.Background(), m.Ref(), func(actor.Context) (message.Result, error) {
		return m.checkName(playerName), nil
	})
	if err != nil {
		log.Error("检查名字失败: %v", err)
		return message.Result_Fail
	}
	return response
}

func (m *UserManager) checkName(playerName string) message.Result {
	if len(playerName) < 2 || len(playerName) > 20 {
		return message.Result_Illegal
	}
	if !m.nameBloomFilter.Contains(playerName) {
		return message.Result_Success
	}
	if duplicate, exists := m.nameCache[playerName]; exists {
		if duplicate {
			return message.Result_Duplicate
		}
		return message.Result_Success
	}
	existingPlayer, err := mongodb.FindOne[player.Player](bson.M{"player_info.player_name": playerName})
	if err != nil {
		log.Error("CheckName query database failed: %v", err)
		return message.Result_Fail
	}
	m.nameCache[playerName] = existingPlayer != nil
	m.nameBloomFilter.Add(playerName)
	if existingPlayer != nil {
		return message.Result_Duplicate
	}
	return message.Result_Success
}

func (m *UserManager) ModifyAvatarSuffix(playerID int64, avatar string) (message.Result, string) {
	type result struct {
		status message.Result
		avatar string
	}
	response, err := actor.Call(context.Background(), m.Ref(), func(execution actor.Context) (result, error) {
		status, appliedAvatar, err := m.players.ModifyAvatarSuffix(execution, playerID, avatar)
		return result{status: status, avatar: appliedAvatar}, err
	})
	if err != nil {
		log.Error("修改玩家 %d 头像失败: %v", playerID, err)
		return message.Result_Fail, ""
	}
	return response.status, response.avatar
}

func (m *UserManager) GetPlayerSnapshot(playerID int64) (player.Snapshot, bool) {
	snapshot, err := m.players.Snapshot(context.Background(), playerID)
	if err != nil {
		if !errors.Is(err, actor.ErrActorStopped) {
			log.Error("获取玩家 %d 快照失败: %v", playerID, err)
		}
		return player.Snapshot{}, false
	}
	return snapshot, true
}

func (m *UserManager) GetPlayerInfo(playerID int64) (*message.PlayerInfo, bool) {
	if snapshot, found := m.GetPlayerSnapshot(playerID); found && snapshot.PlayerInfo != nil {
		return snapshot.PlayerInfo.ToMsg(), true
	}
	offlinePlayer, err := mongodb.FindOneById[player.Player](playerID)
	if err != nil {
		log.Error("获取离线玩家 %d 失败: %v", playerID, err)
		return nil, false
	}
	if offlinePlayer == nil || offlinePlayer.PlayerInfo == nil {
		return nil, false
	}
	return offlinePlayer.PlayerInfo.ToMsg(), true
}

func (m *UserManager) SendToPlayer(playerID int64, msg proto.Message) {
	if err := m.players.SendToClient(context.Background(), playerID, msg); err != nil && !errors.Is(err, actor.ErrActorStopped) {
		log.Error("发送消息给玩家 %d 失败: %v", playerID, err)
	}
}

func (m *UserManager) preloadNames() error {
	players, err := m.loadPlayers()
	if err != nil {
		return fmt.Errorf("preload player names: %w", err)
	}
	for _, playerData := range players {
		if playerData.PlayerInfo != nil && playerData.PlayerInfo.PlayerName != "" {
			m.addNameToCache(playerData.PlayerInfo.PlayerName)
		}
	}
	log.Debug("Preloaded %d names from database", len(m.nameCache))
	return nil
}

func (m *UserManager) addNameToCache(playerName string) {
	m.nameCache[playerName] = true
	m.nameBloomFilter.Add(playerName)
}
