package managers

import (
	"gameserver/common/models"
	"gameserver/common/msg/message"
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
	matchQueue = &match_models.MatchQueue{
		Requests: make(map[int64]*match_models.MatchRequest),
	}
)

// todo type需要用map隔离一下，通过type去获取groupSize
type MatchManager struct {
	actor_manager.ActorMessageHandler `bson:"-"`
}

// Matching 定时任务，每10秒执行一次匹配
func Matching() {
	log.Debug("开始执行匹配任务")

	// 快速复制队列数据，避免长时间持有锁
	requests := matchQueue.CopyRequests()

	if len(requests) == 0 {
		log.Debug("匹配队列为空，跳过本次匹配")
		return
	}

	log.Debug("当前匹配队列中有 %d 个请求", len(requests))

	// 执行匹配逻辑
	matchedGroups := executeMatching(requests)

	// 处理匹配结果
	processMatchResults(matchedGroups)

	log.Debug("匹配任务完成，处理了 %d 个匹配组", len(matchedGroups))

	matchQueue.ProcessTimeoutRequests()
}

// HandleMatch 处理玩家开始匹配请求
func (m *MatchManager) HandleMatch(agent gate.Agent, msg *message.C2S_StartMatch) {
	user := agent.UserData().(models.User)
	player := game.External.UserManager.DirectCaller.GetPlayer(user.PlayerId)
	if player == nil {
		log.Error("玩家不存在: %d", user.PlayerId)
		return
	}
	result := false
	defer func() {
		player.SendToClient(&message.S2C_StartMatch{
			Result: result,
		})
	}()
	// 检查玩家是否已经在匹配队列中
	if matchQueue.IsPlayerInQueue(user.PlayerId) {
		log.Debug("玩家 %d 已经在匹配队列中", user.PlayerId)
		return
	}

	// 创建匹配请求
	matchReq := &match_models.MatchRequest{
		PlayerId:  user.PlayerId,
		TeamId:    player.TeamId,
		MatchType: msg.Type, // 默认匹配类型，可以从消息中获取
		JoinTime:  time.Now(),
	}

	// 加入匹配队列
	matchQueue.AddRequest(matchReq)

	log.Debug("玩家 %d 已加入匹配队列，当前队列大小: %d", user.PlayerId, matchQueue.GetQueueSize())

	// 返回成功结果
	result = true
}

func (m *MatchManager) HandleCancelMatch(agent gate.Agent) {
	user := agent.UserData().(models.User)

	// 从匹配队列中移除玩家
	if matchQueue.RemoveRequest(user.PlayerId) {
		log.Debug("玩家 %d 已从匹配队列中移除", user.PlayerId)
		// 可以发送取消匹配成功的消息
	} else {
		log.Debug("玩家 %d 不在匹配队列中", user.PlayerId)
	}
}

// 匹配算法实现
var groupSize = 8 // 可以根据实际需求调整每组玩家数量
// executeMatching 执行匹配逻辑
func executeMatching(requests []*match_models.MatchRequest) [][]*match_models.MatchRequest {
	var matchedGroups [][]*match_models.MatchRequest

	// 先处理能凑满一组的玩家
	for i := 0; i+groupSize <= len(requests); i += groupSize {
		group := make([]*match_models.MatchRequest, 0, groupSize)
		for j := 0; j < groupSize; j++ {
			group = append(group, requests[i+j])
		}
		matchedGroups = append(matchedGroups, group)
	}

	// 处理剩余不足一组的玩家，补充机器人
	remain := len(requests) % groupSize
	if remain > 0 {
		start := len(requests) - remain
		group := make([]*match_models.MatchRequest, 0, groupSize)
		// 先加入剩余玩家
		for i := start; i < len(requests); i++ {
			group = append(group, requests[i])
		}
		// 需要补充的机器人数量
		needRobots := groupSize - remain
		for i := 0; i < needRobots; i++ {
			robotId := RandomRobotPlayerId()
			robotReq := &match_models.MatchRequest{
				PlayerId:  robotId,
				TeamId:    0,
				MatchType: 0,
				JoinTime:  time.Now(),
				IsRobot:   true,
			}
			group = append(group, robotReq)
			log.Debug("补充机器人进组: %d", robotId)
		}
		matchedGroups = append(matchedGroups, group)
	}

	return matchedGroups
}

// todo 获取机器人id实现
func RandomRobotPlayerId() int64 {
	return 0
}

func RandomRobotPlayerIds(size int) []int64 {
	result := make([]int64, size)
	for i := 0; i < size; i++ {
		result[i] = RandomRobotPlayerId()
	}
	return result
}

// processMatchResults 处理匹配结果
func processMatchResults(matchedGroups [][]*match_models.MatchRequest) {
	for _, group := range matchedGroups {
		if len(group) >= groupSize {
			// 生成房间ID
			r := room.CreateRoom(group)

			// 构建匹配结果消息
			var playerInfos []*message.MatchPlayerInfo
			for _, req := range group {
				playerInfos = append(playerInfos, &message.MatchPlayerInfo{
					PlayerId: req.PlayerId,
					IsRobot:  req.IsRobot,
				})
			}
			// 发送匹配结果给所有玩家
			r.SendRoomMessage(&message.S2C_MatchResult{
				RoomId:      r.RoomId,
				PlayerInfos: playerInfos,
			})

			// 从匹配队列中移除已匹配的玩家
			// 使用批量移除，减少锁的获取次数
			playerIds := make([]int64, len(group))
			for i, req := range group {
				playerIds[i] = req.PlayerId

			}
			matchQueue.RemoveRequests(playerIds)

			log.Debug("成功匹配 %d 个玩家，房间ID: %s", len(group), r.RoomId)
		}
	}
}
