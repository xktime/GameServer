package managers

import (
	"context"
	"errors"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/db/mongodb"
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game/internal/managers/player"
	"gameserver/modules/game/internal/managers/team"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"google.golang.org/protobuf/proto"
)

const managerInitializationRetryDelay = 100 * time.Millisecond

type UserManager struct {
	actor.BaseActor
	players         *player.Registry
	teams           *team.Registry
	memCache        map[string]*models.User
	nameCache       map[string]bool
	nameBloomFilter *utils.BloomFilter
	loadPlayers     func() ([]player.Player, error) `bson:"-"`
	initialized     bool                            `bson:"-"`
}

func NewManagers(ctx context.Context, scope *actor.Scope) (*UserManager, *TeamManager, error) {
	teams, err := team.NewRegistry(scope)
	if err != nil {
		return nil, nil, err
	}
	players, err := player.NewRegistry(scope, teams)
	if err != nil {
		return nil, nil, err
	}
	users, err := NewUserManager(ctx, scope, players, teams)
	if err != nil {
		return nil, nil, err
	}
	teamManager, err := NewTeamManager(ctx, scope, players, teams)
	if err != nil {
		return nil, nil, err
	}
	return users, teamManager, nil
}

func NewUserManager(ctx context.Context, scope *actor.Scope, players *player.Registry, teams *team.Registry) (*UserManager, error) {
	return newUserManager(ctx, scope, players, teams, func() ([]player.Player, error) {
		return mongodb.FindAll[player.Player](bson.M{})
	})
}

func newUserManager(
	ctx context.Context,
	scope *actor.Scope,
	players *player.Registry,
	teams *team.Registry,
	loadPlayers func() ([]player.Player, error),
) (*UserManager, error) {
	if players == nil {
		return nil, fmt.Errorf("game: Player Registry is nil")
	}
	if teams == nil {
		return nil, fmt.Errorf("game: Team Registry is nil")
	}
	if loadPlayers == nil {
		return nil, fmt.Errorf("game: Player loader is nil")
	}
	definition, err := actor.Define(scope, actor.User, func(context.Context, string) (*UserManager, error) {
		manager := &UserManager{
			players:         players,
			teams:           teams,
			memCache:        make(map[string]*models.User),
			nameCache:       make(map[string]bool),
			nameBloomFilter: utils.NewBloomFilter(1000000, 7),
			loadPlayers:     loadPlayers,
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
	for _, user := range m.memCache {
		user.LastOfflineTime = now
		if _, err := mongodb.Save(user); err != nil {
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

func (m *UserManager) doUserLogin(ctx context.Context, agent gate.Agent, openID string, serverID int32, loginType message.LoginType) (*message.S2C_Login, error) {
	accountID := fmt.Sprintf("%d_%s", serverID, openID)
	if existingUser, exists := m.memCache[accountID]; exists {
		log.Debug("UserLogin: user already online (顶号操作): %s", accountID)
		if err := m.doUserOffline(ctx, *existingUser, true); err != nil {
			return nil, err
		}
	}

	user, err := mongodb.FindOne[models.User](bson.M{"OpenId": openID, "ServerId": serverID})
	if err != nil {
		return nil, err
	}
	isNew := user == nil
	loginTime := time.Now().Unix()
	if isNew {
		user = &models.User{
			AccountId:  accountID,
			OpenId:     openID,
			ServerId:   serverID,
			PlayerId:   utils.FlakeId(),
			Platform:   loginType,
			CreateTime: loginTime,
		}
	}
	user.RecordLogin(loginTime)
	agent.SetUserData(*user)
	m.memCache[user.AccountId] = user

	if err := m.players.Login(ctx, agent, *user, isNew); err != nil {
		delete(m.memCache, user.AccountId)
		agent.SetUserData(nil)
		return nil, err
	}
	if _, err := mongodb.Save(user); err != nil {
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

func (m *UserManager) UserOffline(user models.User) {
	if err := actor.Tell(context.Background(), m.Ref(), func(execution actor.Context) error {
		return m.doUserOffline(execution, user, true)
	}); err != nil {
		log.Error("提交用户 %s 下线失败: %v", user.AccountId, err)
	}
}

func (m *UserManager) doUserOffline(ctx context.Context, user models.User, save bool) error {
	offlineErrors := make([]error, 0)
	if save {
		user.LastOfflineTime = time.Now().Unix()
		if _, err := mongodb.Save(user); err != nil {
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
	delete(m.memCache, user.AccountId)
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
