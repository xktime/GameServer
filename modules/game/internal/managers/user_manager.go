package managers

import (
	"fmt"
	"gameserver/common/db/mongodb"
	"gameserver/common/models"
	"gameserver/common/utils"
	actor_manager "gameserver/core/actor"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game/internal/managers/player"
	"gameserver/modules/game/internal/managers/team"
	"sync"
	"time"

	"gopkg.in/mgo.v2/bson"
)

type UserManager struct {
	actor_manager.ActorMessageHandler `bson:"-"`
}

// 全局缓存，使用sync.Map保证并发安全
var (
	memCache    sync.Map // 用户缓存
	playerCache sync.Map // 玩家缓存
)

func (m *UserManager) UserLogin(agent gate.Agent, openId string, serverId int32) {
	// 1. 优先从缓存查找用户（检测顶号操作）
	accountId := fmt.Sprintf("%d_%s", serverId, openId)
	if existingUser := m.getUserFromCache(accountId); existingUser != nil {
		log.Debug("UserLogin: user already online (顶号操作): %s", accountId)
		// 处理顶号逻辑：先让旧用户下线
		m.UserOffline(*existingUser)
	}

	// 2. 从数据库查询用户
	user, err := mongodb.FindOne[models.User](bson.M{"OpenId": openId, "ServerId": serverId})
	if err != nil {
		log.Error("UserLogin find user failed: %v", err)
		return
	}

	isNew := user == nil
	if isNew {
		// 新注册流程
		user = &models.User{
			AccountId: accountId,
			OpenId:    openId,
			ServerId:  serverId,
			PlayerId:  utils.FlakeId(),
			// todo platform需要传进来修改？
			Platform: models.DouYin,
		}
		if _, err := mongodb.Save(user); err != nil {
			log.Error("Failed to save new user [openId: %s, serverId: %d]: %v", openId, serverId, err)
			return
		}
		log.Debug("UserLogin new user: %v", user)
	} else {
		// 老用户流程
		log.Debug("UserLogin old user: %v", user)
	}

	user.LoginTime = time.Now().Unix()
	log.Debug("user login: %s", user.AccountId)

	// 设置用户数据到agent
	agent.SetUserData(*user)

	// 更新缓存
	m.updateUserCache(user)

	// 调用玩家登录
	p := player.Login(agent, isNew)
	m.updatePlayerCache(p)
}

// 玩家下线处理
func (m *UserManager) UserOffline(user models.User) {
	// 先从缓存获取玩家信息
	p := m.getPlayerFromCache(user.PlayerId)
	if p != nil {
		if p.TeamId > 0 {
			team.PlayerOffline(p.TeamId, p.PlayerId)
		}
		// 停止玩家Actor
		actor_manager.StopGroup(actor_manager.Player, p.PlayerId)

		// 清理玩家缓存
		m.removePlayerCache(user.PlayerId)

	}

	// 清理用户缓存
	m.removeUserCache(user.AccountId)

	log.Debug("User offline: %s, PlayerId: %d", user.AccountId, user.PlayerId)
}

// 通过openId和serverId获取用户（优先缓存，后查数据库）
func (m *UserManager) GetUserByOpenId(openId string, serverId int32) (models.User, bool) {
	accountId := fmt.Sprintf("%d_%s", serverId, openId)

	// 1. 优先从缓存获取
	if user := m.getUserFromCache(accountId); user != nil {
		return *user, true
	}

	// 2. 缓存不存在，从数据库查询
	user, err := mongodb.FindOne[models.User](bson.M{"OpenId": openId, "ServerId": serverId})
	if err != nil {
		log.Error("GetUserByOpenId query database failed for openId %s, serverId %d: %v", openId, serverId, err)
		return models.User{}, false
	}

	if user == nil {
		log.Debug("User not found in database for openId %s, serverId %d", openId, serverId)
		return models.User{}, false
	}

	// 3. 查询到用户，更新缓存
	log.Debug("GetUserByOpenId found user in database: %s, updating cache", accountId)
	m.updateUserCache(user)

	return *user, true
}

// 通过accountId获取用户（仅从缓存获取）
func (m *UserManager) GetUser(accountId string) (models.User, bool) {
	if user := m.getUserFromCache(accountId); user != nil {
		return *user, true
	}
	return models.User{}, false
}

// 获取所有在线用户
func (m *UserManager) GetUsers() []models.User {
	var users []models.User
	memCache.Range(func(key, value interface{}) bool {
		if user, ok := value.(*models.User); ok {
			users = append(users, *user)
		}
		return true
	})
	return users
}

// 强制清理所有缓存（用于维护或重启）
func (m *UserManager) ClearAllCache() {
	// 统计清理前的数量
	userCount := 0
	playerCount := 0

	memCache.Range(func(key, value interface{}) bool {
		userCount++
		return true
	})

	playerCache.Range(func(key, value interface{}) bool {
		playerCount++
		return true
	})

	// 清理所有缓存
	memCache = sync.Map{}
	playerCache = sync.Map{}

	log.Debug("Cleared all caches - users: %d, players: %d", userCount, playerCount)
}

// 检查用户是否在线
func (m *UserManager) IsUserOnline(accountId string) bool {
	_, exists := memCache.Load(accountId)
	return exists
}

// 获取用户登录时间
func (m *UserManager) GetUserLoginTime(accountId string) (int64, bool) {
	if value, exists := memCache.Load(accountId); exists {
		if user, ok := value.(*models.User); ok {
			return user.LoginTime, true
		}
	}
	return 0, false
}

// 更新用户缓存
func (m *UserManager) updateUserCache(user *models.User) {
	memCache.Store(user.AccountId, user)
}

// 移除用户缓存
func (m *UserManager) removeUserCache(accountId string) {
	if _, exists := memCache.Load(accountId); exists {
		memCache.Delete(accountId)
		log.Debug("Removed user cache: %s", accountId)
	}
}

// 从缓存获取用户（内部方法）
func (m *UserManager) getUserFromCache(accountId string) *models.User {
	if value, exists := memCache.Load(accountId); exists {
		if user, ok := value.(*models.User); ok {
			return user
		}
	}
	return nil
}

// 更新玩家缓存
func (m *UserManager) updatePlayerCache(playerInstance *player.Player) {
	playerCache.Store(playerInstance.PlayerId, playerInstance)
}

// 从缓存获取玩家
func (m *UserManager) getPlayerFromCache(playerId int64) *player.Player {
	if value, exists := playerCache.Load(playerId); exists {
		if playerInstance, ok := value.(*player.Player); ok {
			return playerInstance
		}
	}
	return nil
}

// 移除玩家缓存
func (m *UserManager) removePlayerCache(playerId int64) {
	if _, exists := playerCache.Load(playerId); exists {
		playerCache.Delete(playerId)
		log.Debug("Removed player cache: %d", playerId)
	}
}

// 获取所有缓存的玩家
func (m *UserManager) GetPlayers() []*player.Player {
	var players []*player.Player
	playerCache.Range(func(key, value interface{}) bool {
		if playerInstance, ok := value.(*player.Player); ok {
			players = append(players, playerInstance)
		}
		return true
	})
	return players
}

// 获取缓存的玩家（优先从缓存获取，缓存没有则从Actor获取）
func (m *UserManager) GetPlayer(playerId int64) *player.Player {
	// 优先从缓存获取
	if cachedPlayer := m.getPlayerFromCache(playerId); cachedPlayer != nil {
		return cachedPlayer
	}

	// 缓存中没有，从Actor获取
	if actorPlayer := actor_manager.Get[player.Player](playerId); actorPlayer != nil {
		// 获取到后更新缓存
		m.updatePlayerCache(actorPlayer)
		return actorPlayer
	}

	return nil
}

// 获取玩家缓存统计信息
func (m *UserManager) GetPlayerCacheStats() map[string]interface{} {
	count := 0
	playerCache.Range(func(key, value interface{}) bool {
		count++
		return true
	})

	return map[string]interface{}{
		"cached_players":    count,
		"player_cache_size": count,
	}
}

// 获取缓存统计信息
func (m *UserManager) GetCacheStats() map[string]interface{} {
	userCount := 0
	playerCount := 0

	memCache.Range(func(key, value interface{}) bool {
		userCount++
		return true
	})

	playerCache.Range(func(key, value interface{}) bool {
		playerCount++
		return true
	})

	return map[string]interface{}{
		"online_users":     userCount,
		"cached_players":   playerCount,
		"total_cache_size": userCount + playerCount,
	}
}
