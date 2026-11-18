package managers

import (
	"gameserver/core/log"
	"time"
)

// 队伍匹配请求结构
type teamMatchRequest struct {
	JoinTime  time.Time // 加入时间
	PlayerIds []int64   // 队伍中的所有玩家ID
	TeamId    int64     // 队伍ID
	IsRobot   bool      // 是否是机器人队伍
	MatchID   string    // 非空表示正在向 Room 结算
}

// 匹配队列结构
type matchQueue struct {
	TeamRequests map[int64]*teamMatchRequest // 队伍编号 → 队伍匹配请求。
}

// newMatchQueue 创建新的匹配队列
func newMatchQueue() *matchQueue {
	return &matchQueue{
		TeamRequests: make(map[int64]*teamMatchRequest),
	}
}

// AddTeamRequest 添加队伍匹配请求到队列
func (q *matchQueue) AddTeamRequest(req *teamMatchRequest) {
	// 添加队伍请求
	q.TeamRequests[req.TeamId] = req

	log.Debug("队伍 %d 加入匹配队列，包含 %d 个玩家", req.TeamId, len(req.PlayerIds))
}

// RemoveTeamRequest 从队列中移除队伍匹配请求
func (q *matchQueue) RemoveTeamRequest(teamId int64) bool {
	if req, exists := q.TeamRequests[teamId]; exists {
		if req.MatchID != "" {
			return false
		}
		// 移除队伍请求
		delete(q.TeamRequests, teamId)
		return true
	}
	return false
}

// IsTeamInQueue 检查队伍是否在匹配队列中
func (q *matchQueue) IsTeamInQueue(teamId int64) bool {
	_, exists := q.TeamRequests[teamId]
	return exists
}

// GetQueueSize 获取队列中的队伍数量
func (q *matchQueue) GetQueueSize() int {
	return len(q.TeamRequests)
}

// GetTotalPlayers 获取队列中的总玩家数量
func (q *matchQueue) GetTotalPlayers() int {
	total := 0
	for _, req := range q.TeamRequests {
		total += len(req.PlayerIds)
	}
	return total
}

// GetTeamRequests 返回尚未进入 Room 结算的请求指针列表，仅在 Match Actor 邮箱内使用。
func (q *matchQueue) GetTeamRequests() []*teamMatchRequest {
	requests := make([]*teamMatchRequest, 0, len(q.TeamRequests))
	for _, req := range q.TeamRequests {
		if req.MatchID == "" {
			requests = append(requests, req)
		}
	}

	return requests
}

func (q *matchQueue) MarkSettling(teamIDs []int64, matchID string) bool {
	if matchID == "" || len(teamIDs) == 0 {
		return false
	}
	for _, teamID := range teamIDs {
		request, exists := q.TeamRequests[teamID]
		if !exists || request.MatchID != "" {
			return false
		}
	}
	for _, teamID := range teamIDs {
		q.TeamRequests[teamID].MatchID = matchID
	}
	return true
}

func (q *matchQueue) RequeueMatch(matchID string) int {
	requeued := 0
	for _, request := range q.TeamRequests {
		if request.MatchID == matchID {
			request.MatchID = ""
			requeued++
		}
	}
	return requeued
}

func (q *matchQueue) RemoveSettledMatch(matchID string) int {
	removed := 0
	for teamID, request := range q.TeamRequests {
		if request.MatchID != matchID {
			continue
		}
		delete(q.TeamRequests, teamID)
		removed++
	}
	return removed
}

func (q *matchQueue) RemoveSettledTeams(matchID string, teamIDs []int64) int {
	removed := 0
	for _, teamID := range teamIDs {
		request, exists := q.TeamRequests[teamID]
		if !exists || request.MatchID != matchID {
			continue
		}
		delete(q.TeamRequests, teamID)
		removed++
	}
	return removed
}
