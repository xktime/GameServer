package models

import (
	"gameserver/core/log"
	"time"
)

// 队伍匹配请求结构
type TeamMatchRequest struct {
	TeamId    int64     `json:"team_id"`    // 队伍ID
	PlayerIds []int64   `json:"player_ids"` // 队伍中的所有玩家ID
	MatchType int32     `json:"match_type"` // 匹配类型
	JoinTime  time.Time `json:"join_time"`  // 加入时间
	IsRobot   bool      `json:"is_robot"`   // 是否是机器人队伍
	TeamSize  int       `json:"team_size"`  // 队伍大小
}

// 匹配队列结构
type MatchQueue struct {
	TeamRequests map[int64]*TeamMatchRequest // teamId -> TeamMatchRequest
	PlayerToTeam map[int64]int64             // playerId -> teamId (用于快速查找玩家所属队伍)
}

// NewMatchQueue 创建新的匹配队列
func NewMatchQueue() *MatchQueue {
	return &MatchQueue{
		TeamRequests: make(map[int64]*TeamMatchRequest),
		PlayerToTeam: make(map[int64]int64),
	}
}

// AddTeamRequest 添加队伍匹配请求到队列
func (q *MatchQueue) AddTeamRequest(req *TeamMatchRequest) {
	// 添加队伍请求
	q.TeamRequests[req.TeamId] = req

	// 建立玩家到队伍的映射关系
	for _, playerId := range req.PlayerIds {
		q.PlayerToTeam[playerId] = req.TeamId
	}

	log.Debug("队伍 %d 加入匹配队列，包含 %d 个玩家", req.TeamId, len(req.PlayerIds))
}

// RemoveTeamRequest 从队列中移除队伍匹配请求
func (q *MatchQueue) RemoveTeamRequest(teamId int64) bool {
	if req, exists := q.TeamRequests[teamId]; exists {
		// 清理玩家到队伍的映射关系
		for _, playerId := range req.PlayerIds {
			delete(q.PlayerToTeam, playerId)
		}

		// 移除队伍请求
		delete(q.TeamRequests, teamId)
		return true
	}
	return false
}

// RemoveTeamRequests 批量移除队伍匹配请求
func (q *MatchQueue) RemoveTeamRequests(teamIds []int64) int {
	removedCount := 0
	for _, teamId := range teamIds {
		if req, exists := q.TeamRequests[teamId]; exists {
			// 清理玩家到队伍的映射关系
			for _, playerId := range req.PlayerIds {
				delete(q.PlayerToTeam, playerId)
			}

			delete(q.TeamRequests, teamId)
			removedCount++
		}
	}

	return removedCount
}

// IsPlayerInQueue 检查玩家是否在匹配队列中
func (q *MatchQueue) IsPlayerInQueue(playerId int64) bool {
	_, exists := q.PlayerToTeam[playerId]
	return exists
}

// IsTeamInQueue 检查队伍是否在匹配队列中
func (q *MatchQueue) IsTeamInQueue(teamId int64) bool {
	_, exists := q.TeamRequests[teamId]
	return exists
}

// GetQueueSize 获取队列中的队伍数量
func (q *MatchQueue) GetQueueSize() int {
	return len(q.TeamRequests)
}

// GetTotalPlayers 获取队列中的总玩家数量
func (q *MatchQueue) GetTotalPlayers() int {
	total := 0
	for _, req := range q.TeamRequests {
		total += len(req.PlayerIds)
	}
	return total
}

// CopyTeamRequests 快速复制队列数据，避免长时间持有锁
func (q *MatchQueue) GetTeamRequests() []*TeamMatchRequest {
	requests := make([]*TeamMatchRequest, 0, len(q.TeamRequests))
	for _, req := range q.TeamRequests {
		requests = append(requests, req)
	}

	return requests
}

// 为了向后兼容，保留原有的玩家匹配请求结构
type MatchRequest struct {
	PlayerId  int64     `json:"player_id"`
	TeamId    int64     `json:"team_id"`
	MatchType int32     `json:"match_type"`
	JoinTime  time.Time `json:"join_time"`
	IsRobot   bool      `json:"is_robot"`
}
