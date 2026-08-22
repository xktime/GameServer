package managers

import (
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
	"math/rand"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// UserManager 使用BaseActor实现，确保缓存操作按顺序执行
type UserManager struct {
	actor.BaseActor
	memCache        map[string]*models.User  // 用户缓存
	playerCache     map[int64]*player.Player // 玩家缓存
	nameCache       map[string]bool          // 名称缓存，key: playerName, value: bool (true表示已存在)
	nameBloomFilter *utils.BloomFilter       // 布隆过滤器，用于快速判断名称是否可能重复
}

var (
	userManager     *UserManager
	userManagerOnce sync.Once
)

func GetUserManager() *UserManager {
	userManagerOnce.Do(func() {
		userManager = actor.RegisterActor[*UserManager](actor.User, "1")
	})
	return userManager
}

func (m *UserManager) Init(args ...any) {
	m.memCache = make(map[string]*models.User)
	m.playerCache = make(map[int64]*player.Player)
	m.nameCache = make(map[string]bool)
	// 假设最多支持100万个名称，误判率控制在1%以内
	m.nameBloomFilter = utils.NewBloomFilter(1000000, 7)
	m.PreloadNames()
}

// Stop 停止UserManager
func (m *UserManager) Stop() {
	m.RemoveActor(m)
}

func (m *UserManager) GetInterval() int {
	return 5
}

// UserLogin 用户登录 - 异步执行
func (m *UserManager) UserLogin(agent gate.Agent, openId string, serverId int32, loginType message.LoginType) *message.S2C_Login {
	result := m.SendTask(func() *message.S2C_Login {
		return m.doUserLogin(agent, openId, serverId, loginType)
	})

	if err, ok := result.(error); ok {
		log.Error("用户登录失败: %v", err)
		return nil
	}

	if loginResp, ok := result.(*message.S2C_Login); ok {
		return loginResp
	}
	return nil
}

// userLoginSync 用户登录的同步实现
func (m *UserManager) doUserLogin(agent gate.Agent, openId string, serverId int32, loginType message.LoginType) *message.S2C_Login {
	// 1. 优先从缓存查找用户（检测顶号操作）
	accountId := fmt.Sprintf("%d_%s", serverId, openId)
	if existingUser, exists := m.getUserFromCache(accountId); exists {
		log.Debug("UserLogin: user already online (顶号操作): %s", accountId)
		// 处理顶号逻辑：先让旧用户下线
		m.doUserOffline(*existingUser, true)
	}

	// 2. 从数据库查询用户
	user, err := mongodb.FindOne[models.User](bson.M{"OpenId": openId, "ServerId": serverId})
	if err != nil {
		log.Error("UserLogin find user failed: %v", err)
		return &message.S2C_Login{
			LoginResult: -1,
		}
	}

	isNew := user == nil
	loginTime := time.Now().Unix()
	if isNew {
		// 新注册流程
		user = &models.User{
			AccountId:  accountId,
			OpenId:     openId,
			ServerId:   serverId,
			PlayerId:   utils.FlakeId(),
			Platform:   loginType,
			CreateTime: loginTime,
		}
	}

	user.RecordLogin(loginTime)
	log.Debug("user login: %s, isNew: %v", user.AccountId, isNew)

	// 设置用户数据到agent
	agent.SetUserData(*user)

	// 更新缓存
	m.updateUserCache(user)

	// 调用玩家登录
	p := player.DoPlayerLogin(agent, isNew)
	if p == nil {
		return &message.S2C_Login{
			LoginResult: -1,
		}
	}
	m.updatePlayerCache(p)
	// 手动保存一下user
	go func() {
		if _, err := mongodb.Save(user); err != nil {
			log.Error("Failed to save new user [openId: %s, serverId: %d]: %v", openId, serverId, err)
		}
	}()
	return &message.S2C_Login{
		LoginResult: 1,
		LoginInfo: &message.LoginInfo{
			OpenId:        user.OpenId,
			LastLoginTime: int32(user.LastOfflineTime),
			TotalDays:     user.TotalLoginDays,
			IsAccept:      false,
		},
	}
}

// ModifyName 修改名称 - 异步执行
func (m *UserManager) ModifyName(playerId int64, name string) (message.Result, string) {
	result := m.SendTask(func() (message.Result, string) {
		return m.doModifyName(playerId, name)
	})

	if err, ok := result.(error); ok {
		log.Error("修改名字失败: %v", err)
		return message.Result_Fail, ""
	}

	if results, ok := result.([]interface{}); ok && len(results) >= 2 {
		if typedResult, ok := results[0].(message.Result); ok {
			if typedName, ok := results[1].(string); ok {
				return typedResult, typedName
			}
		}
	}
	return message.Result_Fail, ""
}

// modifyNameSync 修改名称的同步实现
func (m *UserManager) doModifyName(playerId int64, name string) (message.Result, string) {
	p := m.getPlayerFromCache(playerId)
	if p != nil {
		result := p.ModifyName(name)
		if result == message.Result_Success {
			m.AddNameToCache(name)
			return message.Result_Success, name
		}
		return result, p.PlayerInfo.PlayerName
	}
	return message.Result_Illegal, ""
}

// UserOffline 玩家下线处理 - 异步执行
func (m *UserManager) UserOffline(user models.User) {
	m.SendTaskAsync(func() {
		m.doUserOffline(user, true)
	})
}

// UserOfflineSync 玩家下线处理的同步实现
func (m *UserManager) doUserOffline(user models.User, save bool) {
	if save {
		// user不是actor，需要手动保存
		user.LastOfflineTime = time.Now().Unix()
		mongodb.Save(user)
	}

	// 先从缓存获取玩家信息
	p := m.getPlayerFromCache(user.PlayerId)
	if p != nil {
		// 清理玩家缓存
		m.removePlayerCache(user.PlayerId)

		p.DoSendToClient(&message.S2C_Logout{})

		p.Stop()

		// todo 玩家离线是否需要离开队伍？有可能需要重连房间
		teamInfo, ok := actor.GetActor[team.Team](actor.Team, p.TeamId)
		if ok {
			teamInfo.LeaveTeam(p.PlayerId)
		}
	}

	// 清理用户缓存
	m.removeUserCache(user.AccountId)

	log.Debug("User offline: %s, PlayerId: %d", user.AccountId, user.PlayerId)
}

// CheckName 检查名称 - 异步执行
func (m *UserManager) CheckName(playerName string) message.Result {
	response := m.SendTask(func() message.Result {
		return m.checkNameSync(playerName)
	})

	if err, ok := response.(error); ok {
		log.Error("检查名字失败: %v", err)
		return message.Result_Fail
	}

	if result, ok := response.(message.Result); ok {
		return result
	}
	return message.Result_Fail
}

// checkNameSync 检查名称的同步实现
func (m *UserManager) checkNameSync(playerName string) message.Result {
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
	if value, exists := m.nameCache[playerName]; exists {
		if isDuplicate := value; isDuplicate {
			return message.Result_Duplicate
		} else {
			return message.Result_Success
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
		m.nameCache[playerName] = true
		m.nameBloomFilter.Add(playerName)
		return message.Result_Duplicate
	} else {
		// 名称可用
		m.nameCache[playerName] = false
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

// GetUserByOpenId 通过openId和serverId获取用户 - 异步执行
func (m *UserManager) GetUserByOpenId(openId string, serverId int32) (models.User, bool) {
	result := m.SendTask(func() (models.User, bool) {
		return m.doGetUserByOpenId(openId, serverId)
	})

	if err, ok := result.(error); ok {
		log.Error("获取用户失败: %v", err)
		return models.User{}, false
	}

	if results, ok := result.([]interface{}); ok && len(results) >= 2 {
		if user, ok := results[0].(models.User); ok {
			if exists, ok := results[1].(bool); ok {
				return user, exists
			}
		}
	}
	return models.User{}, false
}

// getUserByOpenIdSync 通过openId和serverId获取用户的同步实现
func (m *UserManager) doGetUserByOpenId(openId string, serverId int32) (models.User, bool) {
	accountId := fmt.Sprintf("%d_%s", serverId, openId)

	// 1. 优先从缓存获取
	if user, exists := m.getUserFromCache(accountId); exists {
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

// GetUser 通过accountId获取用户（仅从缓存获取）
func (m *UserManager) GetUser(accountId string) (models.User, bool) {
	result := m.SendTask(func() (models.User, bool) {
		user, exists := m.getUserFromCache(accountId)
		if user != nil {
			return *user, exists
		}
		return models.User{}, exists
	})

	if err, ok := result.(error); ok {
		log.Error("获取用户失败: %v", err)
		return models.User{}, false
	}

	if results, ok := result.([]interface{}); ok && len(results) >= 2 {
		if user, ok := results[0].(models.User); ok {
			if exists, ok := results[1].(bool); ok {
				return user, exists
			}
		}
	}
	return models.User{}, false

}

func (m *UserManager) GetUsers() []models.User {
	result := m.SendTask(func() []models.User {
		users := []models.User{}
		for _, user := range m.memCache {
			users = append(users, *user)
		}
		return users
	})
	if err, ok := result.(error); ok {
		log.Error("获取所有用户失败: %v", err)
		return []models.User{}
	}

	if users, ok := result.([]models.User); ok {
		return users
	}
	return []models.User{}
}

// ClearAllCache 强制清理所有缓存（用于维护或重启）
func (m *UserManager) ClearAllCache() {
	m.SendTaskAsync(func() {
		m.doClearAllCache()
	})
}

// clearAllCacheSync 强制清理所有缓存的同步实现
func (m *UserManager) doClearAllCache() {
	// 统计清理前的数量
	userCount := 0
	playerCount := 0

	for range m.memCache {
		userCount++
	}

	for range m.playerCache {
		playerCount++
	}

	// 清理所有缓存
	m.memCache = make(map[string]*models.User)
	m.playerCache = make(map[int64]*player.Player)
	log.Debug("Cleared all caches - users: %d, players: %d", userCount, playerCount)
}

// IsUserOnline 检查用户是否在线
func (m *UserManager) IsUserOnline(accountId string) bool {
	result := m.SendTask(func() bool {
		_, exists := m.memCache[accountId]
		return exists
	})
	if err, ok := result.(error); ok {
		log.Error("检查用户存在失败: %v", err)
		return false
	}

	if exists, ok := result.(bool); ok {
		return exists
	}
	return false
}

// 更新用户缓存
func (m *UserManager) updateUserCache(user *models.User) {
	m.memCache[user.AccountId] = user
}

// 移除用户缓存
func (m *UserManager) removeUserCache(accountId string) {
	if _, exists := m.memCache[accountId]; exists {
		delete(m.memCache, accountId)
		log.Debug("Removed user cache: %s", accountId)
	}
}

// 从缓存获取用户（内部方法）
func (m *UserManager) getUserFromCache(accountId string) (*models.User, bool) {
	if user, exists := m.memCache[accountId]; exists {
		return user, true
	}
	return nil, false
}

// 更新玩家缓存
func (m *UserManager) updatePlayerCache(playerInstance *player.Player) {
	m.SendTaskAsync(func() {
		m.playerCache[playerInstance.PlayerId] = playerInstance
	})
}

// 从缓存获取玩家
func (m *UserManager) getPlayerFromCache(playerId int64) *player.Player {
	if playerInstance, exists := m.playerCache[playerId]; exists {
		return playerInstance
	}
	return nil
}

// 移除玩家缓存
func (m *UserManager) removePlayerCache(playerId int64) {
	if _, exists := m.playerCache[playerId]; exists {
		delete(m.playerCache, playerId)
		log.Debug("Removed player cache: %d", playerId)
	}
}

// GetPlayers 获取所有缓存的玩家
func (m *UserManager) GetPlayers() []*player.Player {
	var players []*player.Player
	for _, playerInstance := range m.playerCache {
		players = append(players, playerInstance)
	}
	return players
}

// GetRandomPlayer 获取随机玩家
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

// GetPlayer 获取缓存的玩家（优先从缓存获取，缓存没有则从Actor获取）
func (m *UserManager) GetPlayer(playerId int64) *player.Player {
	result := m.SendTask(func() *player.Player {
		// 优先从缓存获取
		return m.getPlayerFromCache(playerId)
	})
	if err, ok := result.(error); ok {
		log.Error("从缓存获取玩家失败: %v", err)
		return nil
	}

	if player, ok := result.(*player.Player); ok {
		return player
	}
	// 缓存中没有，从Actor获取
	if actorPlayer := player.GetPlayerActor(playerId); actorPlayer != nil {
		// 获取到后更新缓存
		m.updatePlayerCache(actorPlayer)
		return actorPlayer
	}
	return nil
}

// GetOfflinePlayer 获取离线玩家
func (m *UserManager) GetOfflinePlayer(playerId int64) *player.Player {
	player, err := mongodb.FindOneById[player.Player](playerId)
	if err != nil {
		log.Error("获取非在线玩家Info异常: %v, err: %v", playerId, err)
		return nil
	}
	if player == nil {
		log.Error("获取非在线玩家Info玩家数据不存在: %v", playerId)
		return nil
	}
	return player
}

// AddNameToCache 添加名称到缓存（用于预加载或批量导入）
func (m *UserManager) AddNameToCache(playerName string) {
	m.nameCache[playerName] = true
	m.nameBloomFilter.Add(playerName)
}

// RemoveNameFromCache 从名称缓存中移除（用于清理过期数据）
func (m *UserManager) RemoveNameFromCache(playerName string) {
	delete(m.nameCache, playerName)
	// 注意：布隆过滤器不支持删除，这里只清理内存缓存
}

// PreloadNames 预加载名称到缓存（启动时调用，从数据库加载所有已存在的名称）
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

func (m *UserManager) ModifyAvatarSuffix(playerId int64, avatar string) (message.Result, string) {
	p := m.getPlayerFromCache(playerId)
	if p != nil {
		result := p.ModifyAvatarSuffix(avatar)
		return result, p.PlayerInfo.AvatarSuffix
	}
	return message.Result_Illegal, ""
}
