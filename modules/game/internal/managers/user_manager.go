package managers

import (
	"fmt"
	"gameserver/common/db/mongodb"
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	actor_manager "gameserver/core/actor"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game/internal/managers/player"
	"math/rand"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

type UserManager struct {
	actor_manager.ActorMessageHandler `bson:"-"`
	memCache                          sync.Map           // 用户缓存
	playerCache                       sync.Map           // 玩家缓存
	nameCache                         sync.Map           // 名称缓存，key: playerName, value: bool (true表示已存在)
	nameBloomFilter                   *utils.BloomFilter // 布隆过滤器，用于快速判断名称是否可能重复
}

// 初始化布隆过滤器
func (m *UserManager) OnInitData() {
	// 假设最多支持100万个名称，误判率控制在1%以内
	m.nameBloomFilter = utils.NewBloomFilter(1000000, 7)
	m.PreloadNames()
}

func (m *UserManager) UserLogin(agent gate.Agent, openId string, serverId int32, loginType message.LoginType) {
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
			Platform:  loginType,
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

	p.SendToClient(&message.S2C_Login{
		LoginResult: 1,
		PlayerInfo:  p.ToPlayerInfo(),
	})
}

func (m *UserManager) ModifyName(playerId int64, name string) message.Result {
	p := m.getPlayerFromCache(playerId)
	if p != nil {
		result := p.ModifyName(name)
		if result == message.Result_Success {
			m.AddNameToCache(name)
			return message.Result_Success
		}
		return result
	}
	return message.Result_Illegal
}

// 玩家下线处理
func (m *UserManager) UserOffline(user models.User) {
	// 先从缓存获取玩家信息
	p := m.getPlayerFromCache(user.PlayerId)
	if p != nil {
		// 停止玩家Actor
		actor_manager.StopGroup(actor_manager.Player, p.PlayerId)

		// 清理玩家缓存
		m.removePlayerCache(user.PlayerId)

	}

	// 清理用户缓存
	m.removeUserCache(user.AccountId)

	log.Debug("User offline: %s, PlayerId: %d", user.AccountId, user.PlayerId)
}

func (m *UserManager) CheckName(playerName string) message.Result {
	// 1. 校验名称合法性
	if !m.isValidPlayerName(playerName) {
		return message.Result_Illegal
	}

	// 2. 布隆过滤器快速检查（可能误判，但不会漏判）
	if !m.nameBloomFilter.Contains(playerName) {
		// 布隆过滤器显示名称一定不存在，直接返回成功
		return message.Result_Success
	}

	// 3. 检查内存缓存
	if value, exists := m.nameCache.Load(playerName); exists {
		if isDuplicate, ok := value.(bool); ok {
			if isDuplicate {
				return message.Result_Duplicate
			} else {
				return message.Result_Success
			}
		}
	}

	// 4. 缓存未命中，查询数据库
	existingPlayer, err := mongodb.FindOne[player.Player](bson.M{"player_info.player_name": playerName})
	if err != nil {
		log.Error("CheckName query database failed: %v", err)
		return message.Result_Fail
	}

	// 5. 更新缓存和布隆过滤器
	if existingPlayer != nil {
		// 名称已存在
		m.nameCache.Store(playerName, true)
		m.nameBloomFilter.Add(playerName)
		return message.Result_Duplicate
	} else {
		// 名称可用
		m.nameCache.Store(playerName, false)
		m.nameBloomFilter.Add(playerName)
		return message.Result_Success
	}
}

// 校验玩家名称合法性
func (m *UserManager) isValidPlayerName(name string) bool {
	// 名称长度检查（2-20个字符）
	if len(name) < 2 || len(name) > 20 {
		return false
	}
	// todo 还有其他合法性校验
	return true
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
	m.memCache.Range(func(key, value interface{}) bool {
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

	m.memCache.Range(func(key, value interface{}) bool {
		userCount++
		return true
	})

	m.playerCache.Range(func(key, value interface{}) bool {
		playerCount++
		return true
	})

	// 清理所有缓存
	m.memCache = sync.Map{}
	m.playerCache = sync.Map{}
	log.Debug("Cleared all caches - users: %d, players: %d", userCount, playerCount)
}

// 检查用户是否在线
func (m *UserManager) IsUserOnline(accountId string) bool {
	_, exists := m.memCache.Load(accountId)
	return exists
}

// 更新用户缓存
func (m *UserManager) updateUserCache(user *models.User) {
	m.memCache.Store(user.AccountId, user)
}

// 移除用户缓存
func (m *UserManager) removeUserCache(accountId string) {
	if _, exists := m.memCache.Load(accountId); exists {
		m.memCache.Delete(accountId)
		log.Debug("Removed user cache: %s", accountId)
	}
}

// 从缓存获取用户（内部方法）
func (m *UserManager) getUserFromCache(accountId string) *models.User {
	if value, exists := m.memCache.Load(accountId); exists {
		if user, ok := value.(*models.User); ok {
			return user
		}
	}
	return nil
}

// 更新玩家缓存
func (m *UserManager) updatePlayerCache(playerInstance *player.Player) {
	m.playerCache.Store(playerInstance.PlayerId, playerInstance)
}

// 从缓存获取玩家
func (m *UserManager) getPlayerFromCache(playerId int64) *player.Player {
	if value, exists := m.playerCache.Load(playerId); exists {
		if playerInstance, ok := value.(*player.Player); ok {
			return playerInstance
		}
	}
	return nil
}

// 移除玩家缓存
func (m *UserManager) removePlayerCache(playerId int64) {
	if _, exists := m.playerCache.Load(playerId); exists {
		m.playerCache.Delete(playerId)
		log.Debug("Removed player cache: %d", playerId)
	}
}

// 获取所有缓存的玩家
func (m *UserManager) GetPlayers() []*player.Player {
	var players []*player.Player
	m.playerCache.Range(func(key, value interface{}) bool {
		if playerInstance, ok := value.(*player.Player); ok {
			players = append(players, playerInstance)
		}
		return true
	})
	return players
}

func (m *UserManager) GetRandomPlayer(exceptPlayerId []int64) *player.Player {
	// 先筛选出不在exceptPlayerId中的玩家
	var filteredPlayers []*player.Player
	players := m.GetPlayers()
	for _, p := range players {
		exist := false
		for _, id := range exceptPlayerId {
			if p.PlayerId == id {
				exist = true
				break
			}
		}
		if !exist {
			filteredPlayers = append(filteredPlayers, p)
		}
	}
	// 如果没有可选玩家，返回nil
	if len(filteredPlayers) == 0 {
		return nil
	}
	// 随机返回一个玩家
	randIdx := rand.Intn(len(filteredPlayers))
	return filteredPlayers[randIdx]
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
	m.playerCache.Range(func(key, value interface{}) bool {
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
	nameCount := 0

	m.memCache.Range(func(key, value interface{}) bool {
		userCount++
		return true
	})

	m.playerCache.Range(func(key, value interface{}) bool {
		playerCount++
		return true
	})

	m.nameCache.Range(func(key, value interface{}) bool {
		nameCount++
		return true
	})

	return map[string]interface{}{
		"online_users":     userCount,
		"cached_players":   playerCount,
		"cached_names":     nameCount,
		"total_cache_size": userCount + playerCount + nameCount,
	}
}

// 添加名称到缓存（用于预加载或批量导入）
func (m *UserManager) AddNameToCache(playerName string) {
	m.nameCache.Store(playerName, true)
	m.nameBloomFilter.Add(playerName)
}

// 从名称缓存中移除（用于清理过期数据）
func (m *UserManager) RemoveNameFromCache(playerName string) {
	m.nameCache.Delete(playerName)
	// 注意：布隆过滤器不支持删除，这里只清理内存缓存
}

// 预加载名称到缓存（启动时调用，从数据库加载所有已存在的名称）
func (m *UserManager) PreloadNames() {
	log.Debug("Starting to preload player names from database...")

	// 查询所有玩家名称
	players, err := mongodb.FindAll[player.Player](bson.M{})
	if err != nil {
		log.Error("Failed to preload names: %v", err)
		return
	}

	// 批量添加到缓存
	count := 0
	for _, p := range players {
		if p.PlayerInfo != nil && p.PlayerInfo.PlayerName != "" {
			m.AddNameToCache(p.PlayerInfo.PlayerName)
			count++
		}
	}

	log.Debug("Preloaded %d names from database", count)
}
