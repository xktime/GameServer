package managers

import (
	"context"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	"gameserver/core/log"
	"gameserver/modules/match/playerread"
	"gameserver/modules/match/roomaccept"
	"gameserver/modules/room/matchentry"
	"sort"
	"sync"
	"time"
)

// MatchManager 匹配管理器
type MatchManager struct {
	actor      *matchActor
	definition *actor.Definition[*matchActor, string]
	mu         sync.RWMutex
	recoveryMu utils.KeyedMutex[string]
}

func (h *MatchManager) currentActor() *matchActor {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.actor
}

// matchActor 的可变状态只允许在所属邮箱内访问。
type matchActor struct {
	actor.BaseActor
	players         playerread.PlayerReader
	rooms           roomaccept.Acceptor
	newMatchID      func() string
	now             func() time.Time
	matchQueues     map[int32]*matchQueue
	settlements     map[string]*matchSettlement
	nextSyntheticID int64
}

type matchSettlement struct {
	matchType int32
	admission matchentry.Admission
}

// NewMatchManager 在模块作用域内注册单例定义，
// 并返回已完成初始化的实例。
func NewMatchManager(ctx context.Context, scope *actor.Scope, players playerread.PlayerReader, rooms roomaccept.Acceptor, newMatchID func() string, now func() time.Time) (*MatchManager, error) {
	if players == nil {
		return nil, fmt.Errorf("match: PlayerReader is nil")
	}
	if rooms == nil {
		return nil, fmt.Errorf("match: Room Acceptor is nil")
	}
	if newMatchID == nil {
		return nil, fmt.Errorf("match: MatchID generator is nil")
	}
	if now == nil {
		return nil, fmt.Errorf("match: Clock is nil")
	}
	definition, err := actor.Define(scope, func(context.Context, string) (*matchActor, error) {
		return &matchActor{
			players:     players,
			rooms:       rooms,
			newMatchID:  newMatchID,
			now:         now,
			matchQueues: map[int32]*matchQueue{1: newMatchQueue()},
			settlements: make(map[string]*matchSettlement),
		}, nil
	}, actor.DefinitionOptions{QueueCapacity: 1024})
	if err != nil {
		return nil, err
	}
	instance, err := definition.GetOrCreate(ctx, "singleton")
	if err != nil {
		return nil, err
	}
	return &MatchManager{actor: instance, definition: definition}, nil
}

func (handle *MatchManager) OnTimer() {
	m := handle.currentActor()

	if err := actor.Tell(context.Background(), m.Ref(), func(execution actor.Context) error {
		m.matching(execution)
		m.processTimeoutRequests()
		return nil
	}); err != nil {
		log.Error("提交匹配定时任务失败: %v", err)
	}
}

// Matching 定时任务，每10秒执行一次匹配
func (m *matchActor) matching(ctx context.Context) {
	log.Debug("开始执行匹配任务")
	m.retrySettlements(ctx)

	for matchType, q := range m.matchQueues {
		if q.GetQueueSize() == 0 {
			continue
		}
		groups := m.executeTeamMatchingForType(q, matchType)
		if len(groups) == 0 {
			continue
		}
		for _, group := range groups {
			m.startSettlement(ctx, q, matchType, group)
		}
		log.Debug("匹配任务完成，处理了 %d 个匹配组，匹配类型: %d", len(groups), matchType)
	}
}

func (m *matchActor) retrySettlements(ctx context.Context) {
	matchIDs := make([]string, 0, len(m.settlements))
	for matchID := range m.settlements {
		matchIDs = append(matchIDs, matchID)
	}
	sort.Strings(matchIDs)
	for _, matchID := range matchIDs {
		m.settle(ctx, m.settlements[matchID])
	}
}

func (m *matchActor) startSettlement(ctx context.Context, q *matchQueue, matchType int32, group []*teamMatchRequest) {
	teamIDs := make([]int64, 0, len(group))
	teams := make([]matchentry.MatchedTeam, 0, len(group))
	for _, request := range group {
		if !request.IsRobot {
			teamIDs = append(teamIDs, request.TeamId)
		}
		teams = append(teams, matchentry.MatchedTeam{
			TeamID:    request.TeamId,
			PlayerIDs: append([]int64(nil), request.PlayerIds...),
			IsRobot:   request.IsRobot,
		})
	}
	matchID := m.newMatchID()
	if !q.MarkSettling(teamIDs, matchID) {
		return
	}
	settlement := &matchSettlement{
		matchType: matchType,
		admission: matchentry.Admission{MatchID: matchID, Teams: teams},
	}
	m.settlements[matchID] = settlement
	m.settle(ctx, settlement)
}

func (m *matchActor) settle(ctx context.Context, settlement *matchSettlement) {
	result := m.rooms.AcceptMatch(ctx, copyAdmission(settlement.admission))
	switch result.Status {
	case matchentry.Accepted, matchentry.AlreadyAccepted:
		m.matchQueues[settlement.matchType].RemoveSettledMatch(settlement.admission.MatchID)
		delete(m.settlements, settlement.admission.MatchID)
	case matchentry.Rejected:
		queue := m.matchQueues[settlement.matchType]
		queue.RemoveSettledTeams(settlement.admission.MatchID, result.RejectedTeamIDs)
		queue.RequeueMatch(settlement.admission.MatchID)
		delete(m.settlements, settlement.admission.MatchID)
	case matchentry.Retryable:
		return
	}
}

func copyAdmission(admission matchentry.Admission) matchentry.Admission {
	copied := matchentry.Admission{MatchID: admission.MatchID, Teams: make([]matchentry.MatchedTeam, len(admission.Teams))}
	for index, team := range admission.Teams {
		copied.Teams[index] = team
		copied.Teams[index].PlayerIDs = append([]int64(nil), team.PlayerIDs...)
	}
	return copied
}

// HandleMatch 处理队伍开始匹配请求
func (handle *MatchManager) HandleMatch(playerID int64, msg *message.C2S_StartMatch) (int64, *message.S2C_StartMatch) {
	m := handle.currentActor()

	result, err := actor.Command(context.Background(), m.Ref(), func(execution actor.Context) (matchHandleResult, error) {
		teamID, response, err := m.doHandleMatch(execution, playerID, msg)
		return matchHandleResult{teamID: teamID, response: response}, err
	})
	if err != nil {
		log.Error("处理匹配失败: %v", err)
		return 0, nil
	}
	return result.teamID, result.response
}

type matchHandleResult struct {
	teamID   int64
	response *message.S2C_StartMatch
}

// doHandleMatch 处理队伍开始匹配请求的同步实现
func (m *matchActor) doHandleMatch(ctx context.Context, playerID int64, msg *message.C2S_StartMatch) (int64, *message.S2C_StartMatch, error) {
	teamSnapshot, ok, err := m.players.FindOnlineTeam(ctx, playerID)
	if err != nil {
		return 0, nil, err
	}
	if !ok {
		log.Error("无法获取玩家 %d 的队伍信息", playerID)
		return 0, &message.S2C_StartMatch{
			Result: false,
		}, nil
	}

	// 检查玩家是否有队伍
	if teamSnapshot.TeamID == 0 {
		log.Error("玩家 %d 没有队伍，无法开始匹配", playerID)
		return 0, &message.S2C_StartMatch{
			Result: false,
		}, nil
	}

	q := m.matchQueues[msg.Type]
	if q == nil {
		log.Error("匹配队列不合法: %d", msg.Type)
		return 0, &message.S2C_StartMatch{
			Result: false,
		}, nil
	}

	// 检查队伍是否已经在该类型匹配队列中
	if q.IsTeamInQueue(teamSnapshot.TeamID) {
		log.Debug("队伍 %d 已经在匹配队列中", teamSnapshot.TeamID)
		return 0, &message.S2C_StartMatch{
			Result: false,
		}, nil
	}

	// 创建队伍匹配请求
	teamId := teamSnapshot.TeamID
	teamMatchReq := &teamMatchRequest{
		TeamId:    teamId,
		PlayerIds: teamSnapshot.MemberIDs,
		JoinTime:  m.now(),
		IsRobot:   false,
	}

	// 加入对应类型的匹配队列
	q.AddTeamRequest(teamMatchReq)

	log.Debug("队伍 %d 已加入匹配队列(类型:%d)，包含 %d 个玩家，当前队列大小: %d",
		teamId, msg.Type, len(teamSnapshot.MemberIDs), q.GetQueueSize())

	return teamId, &message.S2C_StartMatch{
		Result: true,
	}, nil
}

// HandleCancelMatch 处理取消匹配请求
func (handle *MatchManager) HandleCancelMatch(playerID int64) (int64, *message.S2C_CancelMatch) {
	m := handle.currentActor()

	result, err := actor.Command(context.Background(), m.Ref(), func(execution actor.Context) (cancelMatchResult, error) {
		teamID, response, err := m.doHandleCancelMatch(execution, playerID)
		return cancelMatchResult{teamID: teamID, response: response}, err
	})
	if err != nil {
		log.Error("取消匹配失败: %v", err)
		return 0, nil
	}
	return result.teamID, result.response
}

type cancelMatchResult struct {
	teamID   int64
	response *message.S2C_CancelMatch
}

// doHandleCancelMatch 处理取消匹配请求的同步实现
func (m *matchActor) doHandleCancelMatch(ctx context.Context, playerID int64) (int64, *message.S2C_CancelMatch, error) {
	playerSnapshot, ok, err := m.players.FindOnline(ctx, playerID)
	if err != nil {
		return 0, nil, err
	}
	if !ok {
		log.Error("玩家不存在: %d", playerID)
		return 0, &message.S2C_CancelMatch{
			Result: false,
		}, nil
	}

	// 检查玩家是否有队伍
	if playerSnapshot.TeamID == 0 {
		log.Error("玩家 %d 没有队伍，无法取消匹配", playerID)
		return 0, &message.S2C_CancelMatch{
			Result: false,
		}, nil
	}

	// 从所有类型的匹配队列中尝试移除该队伍
	removed := false
	for _, q := range m.matchQueues {
		if q.RemoveTeamRequest(playerSnapshot.TeamID) {
			removed = true
			break
		}
	}

	if removed {
		log.Debug("队伍 %d 已从匹配队列中移除", playerSnapshot.TeamID)
		return playerSnapshot.TeamID, &message.S2C_CancelMatch{
			Result: true,
		}, nil
	} else {
		log.Debug("队伍 %d 不在任何匹配队列中", playerSnapshot.TeamID)
		return 0, &message.S2C_CancelMatch{
			Result: false,
		}, nil
	}
}

// 根据匹配类型获取目标房间人数
func getTargetRoomSize(matchType int32) int {
	return 5
}

// executeTeamMatchingForType 执行指定类型的队伍匹配逻辑
func (m *matchActor) executeTeamMatchingForType(q *matchQueue, matchType int32) [][]*teamMatchRequest {
	teamRequests := q.GetTeamRequests()
	if len(teamRequests) == 0 {
		return nil
	}
	log.Debug("类型 %d: 当前匹配队列中有 %d 个队伍，总共 %d 个玩家",
		matchType, len(teamRequests), q.GetTotalPlayers())

	targetRoomSize := getTargetRoomSize(matchType)
	var matchedGroups [][]*teamMatchRequest

	// 按队伍大小排序，优先匹配大队伍
	sort.SliceStable(teamRequests, func(i, j int) bool {
		return len(teamRequests[i].PlayerIds) > len(teamRequests[j].PlayerIds)
	})

	// 尝试组合队伍以达到目标房间大小
	currentGroup := make([]*teamMatchRequest, 0)
	currentSize := 0

	for _, teamReq := range teamRequests {
		// 如果当前队伍加入后会超过目标大小，且当前组不为空，则完成当前组
		if currentSize+len(teamReq.PlayerIds) > targetRoomSize && len(currentGroup) > 0 {
			// 尝试用机器人填充当前组
			currentGroup = m.fillGroupWithRobots(currentGroup, currentSize, targetRoomSize)
			matchedGroups = append(matchedGroups, currentGroup)
			currentGroup = make([]*teamMatchRequest, 0)
			currentSize = 0
		}

		// 添加队伍到当前组
		currentGroup = append(currentGroup, teamReq)
		currentSize += len(teamReq.PlayerIds)

		// 如果达到目标大小，完成当前组
		if currentSize >= targetRoomSize {
			matchedGroups = append(matchedGroups, currentGroup)
			currentGroup = make([]*teamMatchRequest, 0)
			currentSize = 0
		}
	}

	// 处理剩余的队伍
	if len(currentGroup) > 0 {
		// 用机器人填充到目标大小
		currentGroup = m.fillGroupWithRobots(currentGroup, currentSize, targetRoomSize)
		matchedGroups = append(matchedGroups, currentGroup)
	}

	return matchedGroups
}

// fillGroupWithRobots 用机器人填充队伍组到目标大小
func (m *matchActor) fillGroupWithRobots(group []*teamMatchRequest, currentSize, targetSize int) []*teamMatchRequest {
	if currentSize >= targetSize {
		return group
	}

	// 计算需要补充的机器人数量
	needRobots := targetSize - currentSize

	// 生成机器人队伍来填充
	robotTeam := m.syntheticRobotTeams(needRobots)
	group = append(group, robotTeam...)
	return group
}

func (m *matchActor) syntheticRobotTeams(needRobots int) []*teamMatchRequest {
	robotTeams := make([]*teamMatchRequest, 0, needRobots)
	for i := 0; i < needRobots; i++ {
		m.nextSyntheticID--
		teamID := m.nextSyntheticID
		m.nextSyntheticID--
		playerID := m.nextSyntheticID
		robotTeam := &teamMatchRequest{
			PlayerIds: []int64{playerID},
			IsRobot:   true,
			TeamId:    teamID,
			JoinTime:  m.now(),
		}
		robotTeams = append(robotTeams, robotTeam)
		log.Debug("生成合成机器人队伍，队伍ID: %d，当前填充数量: %d", robotTeam.TeamId, i)
	}
	return robotTeams
}

// ProcessTimeoutRequests 处理超时的匹配请求
func (m *matchActor) processTimeoutRequests() {
	expiredTime := m.now().Add(-5 * time.Minute)
	for _, q := range m.matchQueues {
		var expiredTeams []int64
		for teamId, req := range q.TeamRequests {
			if req.MatchID == "" && req.JoinTime.Before(expiredTime) {
				expiredTeams = append(expiredTeams, teamId)
			}
		}

		// 移除过期请求
		for _, teamId := range expiredTeams {
			if q.RemoveTeamRequest(teamId) {
				log.Debug("清理过期的匹配请求: 队伍 %d", teamId)
			}
		}

		if len(expiredTeams) > 0 {
			log.Debug("清理了 %d 个过期的匹配请求", len(expiredTeams))
		}
	}
}
