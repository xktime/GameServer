package managers

import (
	"gameserver/common/base/actor"
	gconf "gameserver/common/config/generated"
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game"
	match_models "gameserver/modules/match/internal/models"
	"strconv"
	"sync"
	"time"
)

// MatchManager 匹配管理器
type MatchManager struct {
	*actor.TaskHandler
	matchQueues map[int32]*match_models.MatchQueue `bson:"-"`
}

var (
	matchManager     *MatchManager
	matchManagerOnce sync.Once
)

func GetMatchManager() *MatchManager {
	matchManagerOnce.Do(func() {
		matchManager = &MatchManager{}
		matchManager.Init()
	})
	return matchManager
}

// Init 初始化匹配管理器
func (m *MatchManager) Init() {
	m.matchQueues = make(map[int32]*match_models.MatchQueue)
	configs, _ := gconf.GetAllMatchConfigs()
	for _, config := range configs {
		m.matchQueues[int32(config.Id)] = match_models.NewMatchQueue()
	}

	// 初始化TaskHandler
	m.TaskHandler = actor.InitTaskHandler(actor.Match, "1", m)
	m.TaskHandler.Start()
}

// Stop 停止MatchManager
func (m *MatchManager) Stop() {
	m.TaskHandler.Stop()
}

func (m *MatchManager) GetInterval() int {
	return 10
}

func (m *MatchManager) OnTimer() {
	m.SendTask(func() *actor.Response {
		m.Matching()
		m.ProcessTimeoutRequests()
		return nil
	})
}

// Matching 定时任务，每10秒执行一次匹配
func (m *MatchManager) Matching() {
	log.Debug("开始执行匹配任务")

	// todo 队列优化，每个队列互不干扰，队列删除时跟加入队列的冲突
	for matchType, q := range m.matchQueues {
		if q.GetQueueSize() == 0 {
			continue
		}
		groups := m.executeTeamMatchingForType(q, matchType)
		if len(groups) == 0 {
			continue
		}
		q.ProcessTeamMatchResults(groups)
		log.Debug("匹配任务完成，处理了 %d 个匹配组，匹配类型: %d", len(groups), matchType)
	}
}

// HandleMatch 处理队伍开始匹配请求
func (m *MatchManager) HandleMatch(agent gate.Agent, msg *message.C2S_StartMatch) {
	m.SendTask(func() *actor.Response {
		m.doHandleMatch(agent, msg)
		return nil
	})
}

// doHandleMatch 处理队伍开始匹配请求的同步实现
func (m *MatchManager) doHandleMatch(agent gate.Agent, msg *message.C2S_StartMatch) {
	user := agent.UserData().(models.User)
	player := game.External.UserManager.GetPlayer(user.PlayerId)
	if player == nil {
		log.Error("玩家不存在: %d", user.PlayerId)
		return
	}

	// 检查玩家是否有队伍
	if player.TeamId == 0 {
		log.Error("玩家 %d 没有队伍，无法开始匹配", user.PlayerId)
		player.SendToClient(&message.S2C_StartMatch{
			Result: false,
		})
		return
	}

	q := m.matchQueues[msg.Type]
	if q == nil {
		log.Error("匹配队列不合法: %d", msg.Type)
		player.SendToClient(&message.S2C_StartMatch{
			Result: false,
		})
		return
	}

	// 检查队伍是否已经在该类型匹配队列中
	if q.IsTeamInQueue(player.TeamId) {
		log.Debug("队伍 %d 已经在匹配队列中", player.TeamId)
		player.SendToClient(&message.S2C_StartMatch{
			Result: false,
		})
		return
	}

	// 获取队伍信息
	team := game.External.TeamManager.DirectCaller.GetTeamByPlayerId(user.PlayerId)
	if team == nil {
		log.Error("无法获取玩家 %d 的队伍信息", user.PlayerId)
		player.SendToClient(&message.S2C_StartMatch{
			Result: false,
		})
		return
	}

	// 创建队伍匹配请求
	teamId := player.TeamId
	teamMatchReq := &match_models.TeamMatchRequest{
		TeamId:    teamId,
		PlayerIds: team.TeamMembers,
		MatchType: msg.Type,
		JoinTime:  time.Now(),
		IsRobot:   false,
		TeamSize:  len(team.TeamMembers),
	}

	// 加入对应类型的匹配队列
	q.AddTeamRequest(teamMatchReq)

	log.Debug("队伍 %d 已加入匹配队列(类型:%d)，包含 %d 个玩家，当前队列大小: %d",
		teamId, msg.Type, len(team.TeamMembers), q.GetQueueSize())

	// 通知队伍中的所有玩家匹配已开始
	game.External.TeamManager.SendMessage(teamId, &message.S2C_StartMatch{
		Result: true,
	})
}

// HandleCancelMatch 处理取消匹配请求
func (m *MatchManager) HandleCancelMatch(agent gate.Agent) {
	m.SendTask(func() *actor.Response {
		m.doHandleCancelMatch(agent)
		return nil
	})
}

// doHandleCancelMatch 处理取消匹配请求的同步实现
func (m *MatchManager) doHandleCancelMatch(agent gate.Agent) {
	user := agent.UserData().(models.User)
	player := game.External.UserManager.GetPlayer(user.PlayerId)
	if player == nil {
		log.Error("玩家不存在: %d", user.PlayerId)
		return
	}

	// 检查玩家是否有队伍
	if player.TeamId == 0 {
		log.Error("玩家 %d 没有队伍，无法取消匹配", user.PlayerId)
		return
	}

	// 从所有类型的匹配队列中尝试移除该队伍
	removed := false
	for _, q := range m.matchQueues {
		if q.RemoveTeamRequest(player.TeamId) {
			removed = true
			break
		}
	}

	if removed {
		log.Debug("队伍 %d 已从匹配队列中移除", player.TeamId)
		game.External.TeamManager.SendMessage(player.TeamId, &message.S2C_CancelMatch{
			Result: true,
		})
	} else {
		log.Debug("队伍 %d 不在任何匹配队列中", player.TeamId)
	}
}

// 根据匹配类型获取目标房间人数
func getTargetRoomSize(matchType int32) int {
	cfg, ok := gconf.GetMatchConfig(strconv.Itoa(int(matchType)))
	if ok && cfg != nil {
		return int(cfg.Room)
	}
	log.Error("获取房间数量，匹配类型 %d 不合法", matchType)
	return 0
}

// executeTeamMatchingForType 执行指定类型的队伍匹配逻辑
func (m *MatchManager) executeTeamMatchingForType(q *match_models.MatchQueue, matchType int32) [][]*match_models.TeamMatchRequest {
	teamRequests := q.GetTeamRequests()
	if len(teamRequests) == 0 {
		return nil
	}
	log.Debug("类型 %d: 当前匹配队列中有 %d 个队伍，总共 %d 个玩家",
		matchType, len(teamRequests), q.GetTotalPlayers())

	targetRoomSize := getTargetRoomSize(matchType)
	var matchedGroups [][]*match_models.TeamMatchRequest

	// 按队伍大小排序，优先匹配大队伍
	sortedRequests := sortTeamsBySize(teamRequests)

	// 尝试组合队伍以达到目标房间大小
	currentGroup := make([]*match_models.TeamMatchRequest, 0)
	currentSize := 0

	for _, teamReq := range sortedRequests {
		// 如果当前队伍加入后会超过目标大小，且当前组不为空，则完成当前组
		if currentSize+teamReq.TeamSize > targetRoomSize && len(currentGroup) > 0 {
			// 尝试用机器人填充当前组
			currentGroup = fillGroupWithRobots(currentGroup, currentSize, targetRoomSize)
			matchedGroups = append(matchedGroups, currentGroup)
			currentGroup = make([]*match_models.TeamMatchRequest, 0)
			currentSize = 0
		}

		// 添加队伍到当前组
		currentGroup = append(currentGroup, teamReq)
		currentSize += teamReq.TeamSize

		// 如果达到目标大小，完成当前组
		if currentSize >= targetRoomSize {
			matchedGroups = append(matchedGroups, currentGroup)
			currentGroup = make([]*match_models.TeamMatchRequest, 0)
			currentSize = 0
		}
	}

	// 处理剩余的队伍
	if len(currentGroup) > 0 {
		// 用机器人填充到目标大小
		currentGroup = fillGroupWithRobots(currentGroup, currentSize, targetRoomSize)
		matchedGroups = append(matchedGroups, currentGroup)
	}

	return matchedGroups
}

// fillGroupWithRobots 用机器人填充队伍组到目标大小
func fillGroupWithRobots(group []*match_models.TeamMatchRequest, currentSize, targetSize int) []*match_models.TeamMatchRequest {
	if currentSize >= targetSize {
		return group
	}

	// 计算需要补充的机器人数量
	needRobots := targetSize - currentSize

	// 获取匹配类型（从第一个队伍获取，如果存在的话）
	matchType := int32(0)
	if len(group) > 0 {
		matchType = group[0].MatchType
	}
	allPlayerIds := make([]int64, 0)
	for _, team := range group {
		allPlayerIds = append(allPlayerIds, team.PlayerIds...)
	}
	// 生成机器人队伍来填充
	robotTeam := RandomRobotPlayerIds(matchType, needRobots, allPlayerIds)
	group = append(group, robotTeam...)
	return group
}

// sortTeamsBySize 按队伍大小排序，大队伍优先
func sortTeamsBySize(teams []*match_models.TeamMatchRequest) []*match_models.TeamMatchRequest {
	// 简单的冒泡排序，按队伍大小降序排列
	sorted := make([]*match_models.TeamMatchRequest, len(teams))
	copy(sorted, teams)

	for i := 0; i < len(sorted)-1; i++ {
		for j := 0; j < len(sorted)-i-1; j++ {
			if sorted[j].TeamSize < sorted[j+1].TeamSize {
				sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
			}
		}
	}

	return sorted
}

func RandomRobotPlayerIds(matchType int32, needRobots int, exceptPlayerId []int64) []*match_models.TeamMatchRequest {
	var robotTeams []*match_models.TeamMatchRequest
	for i := 0; i < needRobots; i++ {
		player := game.External.UserManager.GetRandomPlayer(exceptPlayerId)
		if player == nil {
			log.Error("没有找到机器人玩家，当前填充数量: %d", i)
			continue
		}
		playerId := player.PlayerId
		exceptPlayerId = append(exceptPlayerId, playerId)
		robotTeam := &match_models.TeamMatchRequest{
			PlayerIds: []int64{playerId},
			IsRobot:   true,
			TeamSize:  1,
			TeamId:    utils.FlakeId(),
			MatchType: matchType,
			JoinTime:  time.Now(),
		}
		robotTeams = append(robotTeams, robotTeam)
		log.Debug("生成机器人队伍填充，队伍ID: %d，当前填充数量: %d", robotTeam.TeamId, i)
	}
	return robotTeams
}

// ProcessTimeoutRequests 处理超时的匹配请求
func (m *MatchManager) ProcessTimeoutRequests() {
	expiredTime := time.Now().Add(-5 * time.Minute)
	for _, q := range m.matchQueues {
		var expiredTeams []int64
		for teamId, req := range q.TeamRequests {
			if req.JoinTime.Before(expiredTime) {
				expiredTeams = append(expiredTeams, teamId)
			}
		}

		// 移除过期请求
		for _, teamId := range expiredTeams {
			if req, exists := q.TeamRequests[teamId]; exists {
				// 清理玩家到队伍的映射关系
				for _, playerId := range req.PlayerIds {
					delete(q.PlayerToTeam, playerId)
				}
				delete(q.TeamRequests, teamId)
				log.Debug("清理过期的匹配请求: 队伍 %d", teamId)
			}
		}

		if len(expiredTeams) > 0 {
			log.Debug("清理了 %d 个过期的匹配请求", len(expiredTeams))
		}
	}
}
