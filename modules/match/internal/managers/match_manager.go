package managers

import (
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/common/schedule"
	"gameserver/common/utils"
	actor_manager "gameserver/core/actor"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game"
	"gameserver/modules/match/internal/managers/room"
	match_models "gameserver/modules/match/internal/models"
	"time"
)

// MatchManager 匹配管理器
type MatchManager struct {
	actor_manager.ActorMessageHandler `bson:"-"`
	matchQueue                        *match_models.MatchQueue `bson:"-"`
}

// OnInit 初始化排行榜管理器
func (m *MatchManager) OnInitData() {
	log.Debug("初始化匹配管理器")
	m.matchQueue = match_models.NewMatchQueue()
	schedule.RegisterIntervalSchedul(10, GetMatchManager().Matching)
	schedule.RegisterIntervalSchedul(60, GetMatchManager().ProcessTimeoutRequests)
}

// todo actor优化
// Matching 定时任务，每10秒执行一次匹配
func (m *MatchManager) Matching() {
	log.Debug("开始执行匹配任务")

	// 匹配队列为空，跳过本次匹配
	if m.matchQueue.GetQueueSize() == 0 {
		return
	}

	// 执行匹配逻辑
	matchedGroups := m.executeTeamMatching()
	if matchedGroups == nil {
		return
	}

	// 处理匹配结果
	m.processTeamMatchResults(matchedGroups)

	log.Debug("匹配任务完成，处理了 %d 个匹配组", len(matchedGroups))
}

// HandleMatch 处理队伍开始匹配请求
func (m *MatchManager) HandleMatch(agent gate.Agent, msg *message.C2S_StartMatch) {
	user := agent.UserData().(models.User)
	player := game.External.UserManager.DirectCaller.GetPlayer(user.PlayerId)
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

	// 检查队伍是否已经在匹配队列中
	if m.matchQueue.IsTeamInQueue(player.TeamId) {
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
	teamMatchReq := &match_models.TeamMatchRequest{
		TeamId:    player.TeamId,
		PlayerIds: team.TeamMembers,
		MatchType: msg.Type,
		JoinTime:  time.Now(),
		IsRobot:   false,
		TeamSize:  len(team.TeamMembers),
	}

	// 加入匹配队列
	m.matchQueue.AddTeamRequest(teamMatchReq)

	log.Debug("队伍 %d 已加入匹配队列，包含 %d 个玩家，当前队列大小: %d",
		player.TeamId, len(team.TeamMembers), m.matchQueue.GetQueueSize())

	// 通知队伍中的所有玩家匹配已开始
	for _, memberId := range team.TeamMembers {
		if memberPlayer := game.External.UserManager.DirectCaller.GetPlayer(memberId); memberPlayer != nil {
			memberPlayer.SendToClient(&message.S2C_StartMatch{
				Result: true,
			})
		}
	}
}

// HandleCancelMatch 处理取消匹配请求
func (m *MatchManager) HandleCancelMatch(agent gate.Agent) {
	user := agent.UserData().(models.User)
	player := game.External.UserManager.DirectCaller.GetPlayer(user.PlayerId)
	if player == nil {
		log.Error("玩家不存在: %d", user.PlayerId)
		return
	}

	// 检查玩家是否有队伍
	if player.TeamId == 0 {
		log.Error("玩家 %d 没有队伍，无法取消匹配", user.PlayerId)
		return
	}

	// 从匹配队列中移除队伍
	if m.matchQueue.RemoveTeamRequest(player.TeamId) {
		log.Debug("队伍 %d 已从匹配队列中移除", player.TeamId)

		// 通知队伍中的所有玩家匹配已取消
		team := game.External.TeamManager.DirectCaller.GetTeamByPlayerId(user.PlayerId)
		if team != nil {
			for _, memberId := range team.TeamMembers {
				if memberPlayer := game.External.UserManager.DirectCaller.GetPlayer(memberId); memberPlayer != nil {
					memberPlayer.SendToClient(&message.S2C_CancelMatch{
						Result: true,
					})
				}
			}
		}
	} else {
		log.Debug("队伍 %d 不在匹配队列中", player.TeamId)
	}
}

// todo 根据type使用配置获取
// 匹配算法实现 - 以队伍为单位的匹配
var targetRoomSize = 8 // 目标房间大小

// executeTeamMatching 执行队伍匹配逻辑
func (m *MatchManager) executeTeamMatching() [][]*match_models.TeamMatchRequest {
	teamRequests := m.matchQueue.GetTeamRequests()
	if len(teamRequests) == 0 {
		return nil
	}
	log.Debug("当前匹配队列中有 %d 个队伍，总共 %d 个玩家",
		len(teamRequests), m.matchQueue.GetTotalPlayers())
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

// processTeamMatchResults 处理队伍匹配结果
func (m *MatchManager) processTeamMatchResults(matchedGroups [][]*match_models.TeamMatchRequest) {
	for _, group := range matchedGroups {
		if len(group) > 0 {
			// 收集所有玩家ID
			var allPlayerIds []int64
			var teamIds []int64

			for _, teamReq := range group {
				allPlayerIds = append(allPlayerIds, teamReq.PlayerIds...)
				teamIds = append(teamIds, teamReq.TeamId)
			}

			// 生成房间ID
			r := room.CreateRoom(allPlayerIds, teamIds)

			// 构建匹配结果消息
			var playerInfos []*message.MatchPlayerInfo
			for _, teamReq := range group {
				for _, playerId := range teamReq.PlayerIds {
					playerInfos = append(playerInfos, &message.MatchPlayerInfo{
						PlayerId: playerId,
						IsRobot:  teamReq.IsRobot,
					})
				}
			}
			for _, p := range allPlayerIds {
				game.External.TeamManager.JoinRoom(p, r.RoomId)
			}
			// 发送匹配结果给所有玩家
			r.SendRoomMessage(&message.S2C_MatchResult{
				RoomId:      r.RoomId,
				PlayerInfos: playerInfos,
			})

			// 从匹配队列中移除已匹配的队伍
			m.matchQueue.RemoveTeamRequests(teamIds)

			log.Debug("成功匹配 %d 个队伍，包含 %d 个玩家，房间ID: %s",
				len(group), len(allPlayerIds), r.RoomId)
		}
	}
}

func RandomRobotPlayerIds(matchType int32, needRobots int, exceptPlayerId []int64) []*match_models.TeamMatchRequest {
	var robotTeams []*match_models.TeamMatchRequest
	for i := 0; i < needRobots; i++ {
		player := game.External.UserManager.DirectCaller.GetRandomPlayer(exceptPlayerId)
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
	var expiredTeams []int64
	q := m.matchQueue
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
