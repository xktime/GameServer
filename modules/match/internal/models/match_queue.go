package models

import (
	"gameserver/core/log"
	"sync"
	"time"
)

// 匹配请求结构
type MatchRequest struct {
	PlayerId  int64     `json:"player_id"`
	TeamId    int64     `json:"team_id"`
	MatchType int32     `json:"match_type"`
	JoinTime  time.Time `json:"join_time"`
	IsRobot   bool      `json:"is_robot"`
}

// 匹配队列结构
type MatchQueue struct {
	Requests map[int64]*MatchRequest // playerId -> MatchRequest
	mutex    sync.RWMutex
}

// addRequest 添加匹配请求到队列
func (q *MatchQueue) AddRequest(req *MatchRequest) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.Requests[req.PlayerId] = req
}

// removeRequest 从队列中移除匹配请求
func (q *MatchQueue) RemoveRequest(playerId int64) bool {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if _, exists := q.Requests[playerId]; exists {
		delete(q.Requests, playerId)
		return true
	}
	return false
}

// removeRequests 批量移除匹配请求，减少锁的获取次数
func (q *MatchQueue) RemoveRequests(playerIds []int64) int {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	removedCount := 0
	for _, playerId := range playerIds {
		if _, exists := q.Requests[playerId]; exists {
			delete(q.Requests, playerId)
			removedCount++
		}
	}

	return removedCount
}

// isPlayerInQueue 检查玩家是否在匹配队列中
func (q *MatchQueue) IsPlayerInQueue(playerId int64) bool {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	_, exists := q.Requests[playerId]
	return exists
}

// getQueueSize 获取队列大小
func (q *MatchQueue) GetQueueSize() int {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	return len(q.Requests)
}

// copyRequests 快速复制队列数据，避免长时间持有锁
func (q *MatchQueue) CopyRequests() []*MatchRequest {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	requests := make([]*MatchRequest, 0, len(q.Requests))
	for _, req := range q.Requests {
		// 创建请求的副本，避免并发修改问题
		reqCopy := &MatchRequest{
			PlayerId:  req.PlayerId,
			TeamId:    req.TeamId,
			MatchType: req.MatchType,
			JoinTime:  req.JoinTime,
		}
		requests = append(requests, reqCopy)
	}

	return requests
}

// todo 暂时应该不会有超时
// todo 匹配超时也要返回消息处理
func (q *MatchQueue) ProcessTimeoutRequests() {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	expiredTime := time.Now().Add(-5 * time.Minute)
	var expiredPlayers []int64

	for playerId, req := range q.Requests {
		if req.JoinTime.Before(expiredTime) {
			expiredPlayers = append(expiredPlayers, playerId)
		}
	}

	// 移除过期请求
	for _, playerId := range expiredPlayers {
		delete(q.Requests, playerId)
		log.Debug("清理过期的匹配请求: 玩家 %d", playerId)
	}

	if len(expiredPlayers) > 0 {
		log.Debug("清理了 %d 个过期的匹配请求", len(expiredPlayers))
	}
}
