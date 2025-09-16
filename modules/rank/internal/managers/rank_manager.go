package managers

import (
	"gameserver/common/base/actor"
	"gameserver/common/db/mongodb"
	"gameserver/common/msg/message"
	"gameserver/core/log"
	"gameserver/modules/game"
	"gameserver/modules/rank/internal/models"
	"sort"
	"sync"
	"time"
)

var maxCacheSize = 1000

// RankManager 使用TaskHandler实现，确保排行榜操作按顺序执行
type RankManager struct {
	actor.BaseActor

	// 内存缓存
	PersistId int64                                `bson:"_id"`
	RankCache map[models.RankType]*models.RankData `bson:"rank_cache"`
}

var (
	rankManager     *RankManager
	rankManagerOnce sync.Once
)

func GetRankManager() *RankManager {
	rankManagerOnce.Do(func() {
		rankManager = actor.RegisterActor[*RankManager](actor.Rank, "1")
	})
	return rankManager
}

// Init 初始化RankManager
func (m *RankManager) Init(args ...any) {
	// 从数据库加载排行榜数据
	m.loadRankDataFromDB()
}

// Stop 停止RankManager
func (m *RankManager) Stop() {
	m.RemoveActor(m)
}

// GetPersistId 获取持久化ID
func (r RankManager) GetPersistId() interface{} {
	return r.PersistId
}

// HandleUpdateRankData 更新排行榜数据 - 异步执行
func (r *RankManager) HandleUpdateRankData(playerId int64, req *message.C2S_UpdateRankData) *message.S2C_UpdateRankData {
	response := r.SendTask(func() *message.S2C_UpdateRankData {
		return r.doHandleUpdateRankData(playerId, req)
	})
	return response.(*message.S2C_UpdateRankData)
}

// doHandleUpdateRankData 更新排行榜数据的同步实现
func (r *RankManager) doHandleUpdateRankData(playerId int64, req *message.C2S_UpdateRankData) *message.S2C_UpdateRankData {
	player := game.External.UserManager.GetPlayer(playerId)
	if player == nil {
		return &message.S2C_UpdateRankData{Success: false}
	}
	response := &message.S2C_UpdateRankData{Success: true}
	rankType := models.RankType(req.RankType)
	rankData, exists := r.RankCache[rankType]

	if !exists {
		rankData = &models.RankData{
			RankType:   rankType,
			Items:      make([]models.RankItem, 0),
			UpdateTime: time.Now(),
		}
		r.RankCache[rankType] = rankData
	}

	// 查找是否已存在该玩家
	playerIndex := -1
	for i, item := range rankData.Items {
		if item.PlayerId == playerId {
			playerIndex = i
			break
		}
	}

	// 创建新的排行榜项目
	newItem := models.RankItem{
		PlayerId:   playerId,
		PlayerName: player.PlayerInfo.PlayerName,
		Score:      req.Score,
		Avatar:     player.PlayerInfo.Avatar,
		Level:      player.PlayerInfo.Level,
		UpdateTime: time.Now(),
	}

	if playerIndex >= 0 {
		// 更新现有玩家数据
		rankData.Items[playerIndex] = newItem
	} else {
		// 添加新玩家
		rankData.Items = append(rankData.Items, newItem)
	}

	// 重新排序
	r.sortRankData(rankData)

	// 限制缓存大小
	if len(rankData.Items) > maxCacheSize {
		rankData.Items = rankData.Items[:maxCacheSize]
	}

	rankData.UpdateTime = time.Now()

	log.Debug("排行榜数据已更新: 类型=%d, 玩家=%d, 分数=%d", rankType, playerId, req.Score)
	return response
}

// HandleGetRankList 获取排行榜列表 - 异步执行
func (r *RankManager) HandleGetRankList(playerId int64, req *message.C2S_GetRankList) *message.S2C_GetRankList {
	response := r.SendTask(func() *message.S2C_GetRankList {
		return r.doHandleGetRankList(playerId, req)
	})

	return response.(*message.S2C_GetRankList)
}

// doHandleGetRankList 获取排行榜列表的同步实现
func (r *RankManager) doHandleGetRankList(playerId int64, req *message.C2S_GetRankList) *message.S2C_GetRankList {
	player := game.External.UserManager.GetPlayer(playerId)
	if player == nil {
		return nil
	}
	// 参数验证
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20 // 默认每页20条
	}
	response := &message.S2C_GetRankList{
		RankType:    req.RankType,
		RankItems:   make([]*message.RankItem, 0),
		TotalCount:  0,
		CurrentPage: req.Page,
	}
	rankData, exists := r.RankCache[models.RankType(req.RankType)]
	if !exists {
		return nil
	}

	totalCount := int32(len(rankData.Items))

	// 分页处理
	start := (req.Page - 1) * req.PageSize
	end := start + req.PageSize

	if start >= totalCount {
		response.RankItems = make([]*message.RankItem, 0)
		response.TotalCount = totalCount
		response.CurrentPage = req.Page
		return response
	}

	if end > totalCount {
		end = totalCount
	}

	items := rankData.Items[start:end]

	response.RankItems = make([]*message.RankItem, 0)
	for i, item := range items {
		response.RankItems = append(response.RankItems, &message.RankItem{
			PlayerId:   item.PlayerId,
			PlayerName: item.PlayerName,
			Rank:       int32(i + 1),
			Score:      item.Score,
			Avatar:     item.Avatar,
			Level:      item.Level,
		})
	}
	return response
}

// HandleGetMyRank 获取我的排名 - 异步执行
func (r *RankManager) HandleGetMyRank(playerId int64, rankType int32) *message.S2C_GetMyRank {
	response := r.SendTask(func() *message.S2C_GetMyRank {
		return r.doHandleGetMyRank(playerId, rankType)
	})

	return response.(*message.S2C_GetMyRank)
}

// doHandleGetMyRank 获取我的排名的同步实现
func (r *RankManager) doHandleGetMyRank(playerId int64, rankType int32) *message.S2C_GetMyRank {
	player := game.External.UserManager.GetPlayer(playerId)
	response := &message.S2C_GetMyRank{RankType: rankType}
	if player == nil {
		return response
	}
	rankData, exists := r.RankCache[models.RankType(rankType)]
	if !exists {
		return response
	}

	// 查找玩家排名
	for i, item := range rankData.Items {
		if item.PlayerId == playerId {
			response.MyRank = int32(i + 1)
			response.MyScore = item.Score
		}
	}
	response.TotalCount = int32(len(rankData.Items))
	return response
}

// sortRankData 对排行榜数据进行排序
func (r *RankManager) sortRankData(rankData *models.RankData) {
	sort.Slice(rankData.Items, func(i, j int) bool {
		// 根据排行榜类型进行不同的排序
		switch rankData.RankType {
		case models.RankTypeLevel, models.RankTypePower, models.RankTypeWealth:
			// 分数高的排在前面
			if rankData.Items[i].Score != rankData.Items[j].Score {
				return rankData.Items[i].Score > rankData.Items[j].Score
			}
			// 分数相同时，按更新时间排序（老的排在前面）
			return rankData.Items[j].UpdateTime.After(rankData.Items[i].UpdateTime)
		default:
			return false
		}
	})
}

// loadRankDataFromDB 从数据库加载排行榜数据
func (r *RankManager) loadRankDataFromDB() {
	// 这里可以从数据库加载排行榜数据
	// 暂时使用空数据，实际项目中应该从数据库加载
	log.Debug("排行榜管理器初始化完成")
	data, err := mongodb.FindOneById[RankManager](r.GetPersistId())
	if err != nil {
		log.Error("从数据库加载排行榜数据失败: %v", err)
		return
	}
	if data == nil {
		r.RankCache = make(map[models.RankType]*models.RankData)
	} else {
		r.RankCache = data.RankCache
	}
	r.PersistId = 1 // 使用固定ID，因为现在使用单例模式
	log.Debug("从数据库加载排行榜数据: %v", r)
}
