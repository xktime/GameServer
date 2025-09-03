package models

import (
	"gameserver/common/msg/message"
	"gameserver/core/log"
	"gameserver/modules/game"
	"gameserver/modules/match/internal/managers/room"
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

// processTeamMatchResults 处理队伍匹配结果（指定队列）
func (q *MatchQueue) ProcessTeamMatchResults(matchedGroups [][]*TeamMatchRequest) {
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
			room.SendRoomMessage(r.RoomId, &message.S2C_MatchResult{
				RoomId:      r.RoomId,
				PlayerInfos: playerInfos,
			})

			// 从匹配队列中移除已匹配的队伍
			q.RemoveTeamRequests(teamIds)

			log.Debug("成功匹配 %d 个队伍，包含 %d 个玩家，房间ID: %s",
				len(group), len(allPlayerIds), r.RoomId)
		}
	}
}

// 为了向后兼容，保留原有的玩家匹配请求结构
type MatchRequest struct {
	PlayerId  int64     `json:"player_id"`
	TeamId    int64     `json:"team_id"`
	MatchType int32     `json:"match_type"`
	JoinTime  time.Time `json:"join_time"`
	IsRobot   bool      `json:"is_robot"`
}
