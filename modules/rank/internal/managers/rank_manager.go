package managers

import (
	"gameserver/common/db/mongodb"
	"gameserver/common/msg/message"
	actor_manager "gameserver/core/actor"
	"gameserver/core/log"
	"gameserver/modules/game"
	"gameserver/modules/rank/internal/models"
	"sort"
	"time"
)

var maxCacheSize = 1000

type RankManager struct {
	actor_manager.ActorMessageHandler `bson:"-"`

	// 内存缓存
	PersistId int64                                `bson:"_id"`
	RankCache map[models.RankType]*models.RankData `bson:"rank_cache"`
}

// GetPersistId 获取持久化ID
func (r RankManager) GetPersistId() interface{} {
	return r.PersistId
}

// todo handler需要整理
// NewRankManager 创建新的排行榜管理器

// OnInit 初始化排行榜管理器
func (r *RankManager) OnInitData() {
	// 从数据库加载排行榜数据
	r.loadRankDataFromDB()
}

// UpdateRankData 更新排行榜数据
func (r *RankManager) HandleUpdateRankData(playerId int64, req *message.C2S_UpdateRankData) {
	player := game.External.UserManager.GetPlayer(playerId)
	if player == nil {
		return
	}
	response := &message.S2C_UpdateRankData{Success: true}
	defer player.SendToClient(response)
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

	// todo 玩家信息实现之后需要实现这里
	// 创建新的排行榜项目
	newItem := models.RankItem{
		PlayerId: playerId,
		// PlayerName: player.PlayerInfo.PlayerName,
		Score: req.Score,
		// Avatar:     player.PlayerInfo.Avatar,
		// Level:      player.PlayerInfo.Level,
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
}

// GetRankList 获取排行榜列表
func (r *RankManager) HandleGetRankList(playerId int64, req *message.C2S_GetRankList) {
	player := game.External.UserManager.GetPlayer(playerId)
	if player == nil {
		return
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
	defer player.SendToClient(response)
	rankData, exists := r.RankCache[models.RankType(req.RankType)]
	if !exists {
		return
	}

	totalCount := int32(len(rankData.Items))

	// 分页处理
	start := (req.Page - 1) * req.PageSize
	end := start + req.PageSize

	if start >= totalCount {
		response.RankItems = make([]*message.RankItem, 0)
		response.TotalCount = totalCount
		response.CurrentPage = req.Page
		return
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
}

// GetMyRank 获取我的排名
func (r *RankManager) HandleGetMyRank(playerId int64, rankType int32) {
	player := game.External.UserManager.GetPlayer(playerId)
	if player == nil {
		return
	}
	response := &message.S2C_GetMyRank{RankType: rankType}
	defer player.SendToClient(response)
	rankData, exists := r.RankCache[models.RankType(rankType)]
	if !exists {
		return
	}

	// 查找玩家排名
	for i, item := range rankData.Items {
		if item.PlayerId == playerId {
			response.MyRank = int32(i + 1)
			response.MyScore = item.Score
			response.TotalCount = int32(len(rankData.Items))
			return
		}
	}
	response.TotalCount = int32(len(rankData.Items))
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
			// 分数相同时，按更新时间排序（新的排在前面）
			return rankData.Items[i].UpdateTime.After(rankData.Items[j].UpdateTime)
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
	r.PersistId = GetRankManagerActorId()
	log.Debug("从数据库加载排行榜数据: %v", r)
}
