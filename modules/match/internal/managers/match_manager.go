package managers

import (
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	actor_manager "gameserver/core/actor"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game"
	"gameserver/modules/match/internal/managers/room"
	match_models "gameserver/modules/match/internal/models"
	"time"
)

// 全局匹配队列
var (
	matchQueue = match_models.NewMatchQueue()
)

// MatchManager 匹配管理器
type MatchManager struct {
	actor_manager.ActorMessageHandler `bson:"-"`
}

// Matching 定时任务，每10秒执行一次匹配
func Matching() {
	log.Debug("开始执行匹配任务")

	// 快速复制队列数据，避免长时间持有锁
	teamRequests := matchQueue.CopyTeamRequests()

	if len(teamRequests) == 0 {
		log.Debug("匹配队列为空，跳过本次匹配")
	} else {
		log.Debug("当前匹配队列中有 %d 个队伍，总共 %d 个玩家",
			len(teamRequests), matchQueue.GetTotalPlayers())

		// 执行匹配逻辑
		matchedGroups := executeTeamMatching(teamRequests)

		// 处理匹配结果
		processTeamMatchResults(matchedGroups)

		log.Debug("匹配任务完成，处理了 %d 个匹配组", len(matchedGroups))
	}

	// 处理超时请求
	matchQueue.ProcessTimeoutRequests()
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
	if matchQueue.IsTeamInQueue(player.TeamId) {
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
	matchQueue.AddTeamRequest(teamMatchReq)

	log.Debug("队伍 %d 已加入匹配队列，包含 %d 个玩家，当前队列大小: %d",
		player.TeamId, len(team.TeamMembers), matchQueue.GetQueueSize())

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
	if matchQueue.RemoveTeamRequest(player.TeamId) {
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

// 匹配算法实现 - 以队伍为单位的匹配
var targetRoomSize = 8 // 目标房间大小

// executeTeamMatching 执行队伍匹配逻辑
func executeTeamMatching(teamRequests []*match_models.TeamMatchRequest) [][]*match_models.TeamMatchRequest {
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
		// 如果剩余队伍数量足够，尝试补充机器人
		if currentSize >= targetRoomSize/2 { // 至少达到一半大小
			// 用机器人填充到目标大小
			currentGroup = fillGroupWithRobots(currentGroup, currentSize, targetRoomSize)
			matchedGroups = append(matchedGroups, currentGroup)
		}
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

	// 生成机器人队伍来填充
	for i := 0; i < needRobots; i++ {
		robotTeam := RandomRobotPlayerId(matchType)
		group = append(group, robotTeam)
		log.Debug("生成机器人队伍填充，队伍ID: %d，当前组大小: %d", robotTeam.TeamId, currentSize+i+1)
	}

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
func processTeamMatchResults(matchedGroups [][]*match_models.TeamMatchRequest) {
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
				game.External.TeamManager.DirectCaller.JoinRoom(p, r.RoomId)
			}
			// 发送匹配结果给所有玩家
			r.SendRoomMessage(&message.S2C_MatchResult{
				RoomId:      r.RoomId,
				PlayerInfos: playerInfos,
			})

			// 从匹配队列中移除已匹配的队伍
			matchQueue.RemoveTeamRequests(teamIds)

			log.Debug("成功匹配 %d 个队伍，包含 %d 个玩家，房间ID: %s",
				len(group), len(allPlayerIds), r.RoomId)
		}
	}
}

// todo 随机playerId
func RandomRobotPlayerId(matchType int32) *match_models.TeamMatchRequest {
	return &match_models.TeamMatchRequest{
		PlayerIds: []int64{0},
		IsRobot:   true,
		TeamSize:  1,
		TeamId:    utils.FlakeId(),
		MatchType: matchType,
		JoinTime:  time.Now(),
	}
}
