package managers

import (
	"gameserver/common/base/actor"
	"gameserver/common/db/mongodb"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	"gameserver/core/log"
	"gameserver/modules/rank/internal/models"
	"gameserver/modules/rank/playerread"
	"math"
	"strconv"
	"sync"
	"time"
)

var maxCacheSize = 1000

var types = []message.RankType{
	message.RankType_RankType_LadderPoint,
	message.RankType_RankType_PowerPoint,
	message.RankType_RankType_ChallengePoint,
}

// RankManager 使用TaskHandler实现，确保排行榜操作按顺序执行
type RankManager struct {
	actor.BaseActor
	players playerread.PlayerReader

	// 内存缓存
	PersistId          int64                                             `bson:"_id"`
	RankCache          map[message.RankType]map[int32]*models.RankData   `bson:"rank_cache"`           // rankType -> season -> rankData
	ChallengeCodeCache map[int32]*models.ChallengeCode                   `bson:"challenge_code_cache"` // code -> challengeCode
	CodeIndex          int32                                             `bson:"code_index"`           // 挑战码索引
	rankListCache      map[int64]map[message.RankType][]*models.RankItem `-`                           // playerId -> rankType -> rankData
}

type rankManagerFactory func(playerread.PlayerReader) *RankManager

type rankManagerRegistry struct {
	mu      sync.Mutex
	started bool
	ready   chan struct{}
	manager *RankManager
	failure any
}

var (
	rankManagerRegistration                    = &rankManagerRegistry{}
	registerRankActor       rankManagerFactory = func(players playerread.PlayerReader) *RankManager {
		return actor.RegisterActor[*RankManager](actor.Rank, "1", players)
	}
)

func (r *rankManagerRegistry) register(create func() *RankManager) (manager *RankManager) {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		panic("rank: RegisterRankManager called more than once")
	}
	r.started = true
	r.ready = make(chan struct{})
	r.mu.Unlock()

	defer func() {
		failure := recover()
		r.mu.Lock()
		if failure == nil {
			r.manager = manager
		} else {
			r.failure = failure
		}
		close(r.ready)
		r.mu.Unlock()
		if failure != nil {
			panic(failure)
		}
	}()

	manager = create()
	return manager
}

func (r *rankManagerRegistry) get() *RankManager {
	r.mu.Lock()
	if !r.started {
		r.mu.Unlock()
		panic("rank: GetRankManager called before RegisterRankManager")
	}
	ready := r.ready
	r.mu.Unlock()

	<-ready
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failure != nil {
		panic(r.failure)
	}
	return r.manager
}

func RegisterRankManager(players playerread.PlayerReader) *RankManager {
	return rankManagerRegistration.register(func() *RankManager {
		if players == nil {
			panic("rank: RegisterRankManager requires PlayerReader")
		}
		return registerRankActor(players)
	})
}

func GetRankManager() *RankManager {
	return rankManagerRegistration.get()
}

// Init 初始化RankManager
func (m *RankManager) Init(args ...any) {
	if len(args) != 1 {
		panic("rank: RankManager.Init requires PlayerReader")
	}
	players, ok := args[0].(playerread.PlayerReader)
	if !ok || players == nil {
		panic("rank: RankManager.Init received invalid PlayerReader")
	}
	m.players = players

	// 从数据库加载排行榜数据
	m.loadRankDataFromDB()

	// 重启之后重置一下挑战码
	m.ChallengeCodeCache = make(map[int32]*models.ChallengeCode)
	m.CodeIndex = models.ChallengeCodeFloorint

	seasonManager := GetSeasonManager()
	season := seasonManager.GetCurrentSeason()
	// 初始化新排行榜类型
	for _, rankType := range types {
		if _, ok := m.RankCache[rankType]; ok {
			continue
		}
		m.RankCache[rankType] = make(map[int32]*models.RankData)
		rankData := &models.RankData{
			Season:     season,
			RankType:   rankType,
			Items:      make([]*models.RankItem, 0),
			UpdateTime: time.Now(),
		}
		if m.isSeasonType(rankType) {
			m.RankCache[rankType][season] = rankData
		} else {
			m.RankCache[rankType][0] = rankData
		}
	}
}

// Stop 停止RankManager
func (m *RankManager) Stop() {
	m.RemoveActor(m)
}

// GetPersistId 获取持久化ID
func (r RankManager) GetPersistId() interface{} {
	return r.PersistId
}

func (r *RankManager) OnCrossDay(season int32) {
	for t, rankDatas := range r.RankCache {
		if r.isSeasonType(t) {
			rankDatas[season].OnCrossDay()
		} else {
			rankDatas[0].OnCrossDay()
		}
	}
	// 只保留当前赛季和上个赛季
	rank := r.RankCache[message.RankType_RankType_LadderPoint]
	if rank != nil {
		for i := int32(0); i < season-1; i++ {
			delete(rank, i)
		}
	}
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
	playerSnapshot, ok := r.players.FindOnline(playerId)
	if !ok {
		return &message.S2C_UpdateRankData{Success: false}
	}

	response := &message.S2C_UpdateRankData{Success: true}
	rankType := message.RankType(req.RankType)
	rankData := r.GetRankData(rankType)

	// 查找是否已存在该玩家
	playerIndex := rankData.GetRankItemIndex(playerId)

	// 创建新的排行榜项目
	newItem := &models.RankItem{
		PlayerId:   playerId,
		PlayerName: playerSnapshot.Name,
		Score:      int64(req.Score),
		Avatar:     playerSnapshot.AvatarURL,
		Level:      playerSnapshot.Level,
		UpdateTime: time.Now(),
		OtherInfos: make([]*message.OtherInfo, 0),
	}

	if playerIndex >= 0 {
		// 更新现有玩家数据
		if rankType == message.RankType_RankType_PowerPoint {
			r.UpdatePower(playerId, float64(newItem.Score))
			newItem.UpdatePower(float64(newItem.Score))
		}
		rankData.Items[playerIndex] = newItem
	} else {
		// 添加新玩家
		newItem.UpdatePower(490)
		rankData.Items = append(rankData.Items, newItem)
	}

	// 重新排序
	rankData.SortRankData()

	// 限制缓存大小
	if len(rankData.Items) > maxCacheSize {
		rankData.Items = rankData.Items[:maxCacheSize]
	}

	rankData.UpdateTime = time.Now()

	log.Debug("排行榜数据已更新: 类型=%d, 玩家=%d, 分数=%d", rankType, playerId, req.Score)
	return response
}

func (m *RankManager) GetChallengeList(playerId int64, req *message.C2S_GetChanllengeList) *message.S2C_GetChanllengeList {
	response := m.SendTask(func() *message.S2C_GetChanllengeList {
		return m.doHandleGetChallengeList(playerId, req)
	})
	return response.(*message.S2C_GetChanllengeList)
}

func (m *RankManager) doHandleGetChallengeList(playerId int64, req *message.C2S_GetChanllengeList) *message.S2C_GetChanllengeList {
	response := &message.S2C_GetChanllengeList{
		RankItem: make([]*message.RankItem, 0),
	}
	rankType := message.RankType(req.RankType)
	if req.RefreshType == message.RefreshType_RefreshType_None {
		if !m.hasRankListCache(playerId, rankType) {
			m.RefreshRankList(playerId, rankType)
		}
	} else {
		m.RefreshRankList(playerId, rankType)
	}
	if items, ok := m.rankListCache[playerId][rankType]; ok {
		for _, item := range items {
			response.RankItem = append(response.RankItem, item.ToMsg())
		}
	}
	return response
}

func (m *RankManager) hasRankListCache(playerId int64, rankType message.RankType) bool {
	if _, ok := m.rankListCache[playerId]; ok {
		if _, ok := m.rankListCache[playerId][rankType]; ok {
			return true
		}
	}
	return false
}

func (m *RankManager) RefreshRankList(playerId int64, rankType message.RankType) []*models.RankItem {
	result := make([]*models.RankItem, 0)
	rankDatas := m.GetRankData(rankType)

	// 找到玩家在排行榜中的位置
	playerIndex := rankDatas.GetRankItemIndex(playerId)
	items := rankDatas.Items

	if playerIndex == -1 {
		playerIndex = int32(len(items) - 1)
	}
	// 收集可挑战的玩家
	candidates := make([]*models.RankItem, 0)

	// 获取前面的玩家：最多取前30名
	frontCount := 30
	frontStart := int(math.Max(0, float64(playerIndex-int32(frontCount))))
	frontEnd := playerIndex
	candidates = append(candidates, items[frontStart:frontEnd]...)

	// 获取后面的玩家：最多取后20名
	backCount := 20
	backStart := playerIndex + 1
	backEnd := int(math.Min(float64(len(items)), float64(playerIndex+int32(backCount+1))))
	candidates = append(candidates, items[backStart:backEnd]...)

	// 选6位，排除自己
	num := int32(5)
	var indexs = utils.RandByArrayCount(candidates, num+1)

	// 转换为响应格式
	for _, index := range indexs {
		if int32(len(result)) >= num || candidates[index].PlayerId == playerId {
			continue
		}
		result = append(result, candidates[index])
	}
	if _, ok := m.rankListCache[playerId]; !ok {
		m.rankListCache[playerId] = make(map[message.RankType][]*models.RankItem)
	}
	m.rankListCache[playerId][rankType] = result
	return result
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
	if _, ok := r.players.FindOnline(playerId); !ok {
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
		Season:      req.Season,
	}
	rankData := r.GetRankData(message.RankType(req.RankType))

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
	for _, item := range items {
		response.RankItems = append(response.RankItems, item.ToMsg())

	}
	return response
}

func (r *RankManager) GeneratorChallengeCode(req *message.C2S_GeneratorChanllengeCode) *message.S2C_GeneratorChanllengeCode {
	response := r.SendTask(func() *message.S2C_GeneratorChanllengeCode {
		return r.doGeneratorChallengeCode(req)
	})

	return response.(*message.S2C_GeneratorChanllengeCode)
}

// doGeneratorChallengeCode 生成挑战码的同步实现
func (r *RankManager) doGeneratorChallengeCode(req *message.C2S_GeneratorChanllengeCode) *message.S2C_GeneratorChanllengeCode {
	// 如果code大于celing需要清理，重置
	// index++ 生成一个
	if r.CodeIndex >= models.ChallengeCodeCeilint {
		r.CodeIndex = models.ChallengeCodeFloorint
		r.clearExpiredCodes()
	}
	r.CodeIndex++
	now := time.Now().Unix()
	playerId, _ := strconv.ParseInt(req.PlayerId, 10, 64)
	chanllengeCode := &models.ChallengeCode{
		PlayerId:     playerId,
		Code:         r.CodeIndex,
		ExpireTime:   now + int64(models.ChallengeCodeExpireTime),
		GenerateTime: now,
	}
	r.ChallengeCodeCache[r.CodeIndex] = chanllengeCode

	item := r.getRankItem(playerId)
	if item.OtherInfos == nil {
		item.OtherInfos = make([]*message.OtherInfo, 0)
	}
	item.OtherInfos = append(item.OtherInfos, &message.OtherInfo{
		Key:   "chanllenge_key",
		Value: strconv.FormatInt(int64(chanllengeCode.Code), 10),
	})
	return &message.S2C_GeneratorChanllengeCode{
		Code:            chanllengeCode.Code,
		ExpireTimestamp: int32(chanllengeCode.ExpireTime),
	}
}

func (r *RankManager) clearExpiredCodes() {
	now := time.Now().Unix()
	for code, c := range r.ChallengeCodeCache {
		if c.ExpireTime < now {
			item := r.getRankItem(c.PlayerId)
			if item.OtherInfos == nil {
				continue
			}
			for _, info := range item.OtherInfos {
				if info.Key == "chanllenge_key" {
					info.Value = ""
					continue
				}
			}
			delete(r.ChallengeCodeCache, code)
		}
	}
}

func (r *RankManager) ChanllengeByCode(req *message.C2S_ChanllengeByCode) *message.S2C_ChanllengeByCode {
	response := r.SendTask(func() *message.S2C_ChanllengeByCode {
		return r.doChanllengeByCode(req)
	})

	return response.(*message.S2C_ChanllengeByCode)
}

// doChanllengeByCode 挑战码验证的同步实现
func (r *RankManager) doChanllengeByCode(req *message.C2S_ChanllengeByCode) *message.S2C_ChanllengeByCode {
	codeInfo := r.ChallengeCodeCache[req.Code]
	if codeInfo == nil {
		log.Debug("挑战码不存在: 挑战码=%d", req.Code)
		return &message.S2C_ChanllengeByCode{
			RankItem: nil,
			Result:   message.ChanllengeResult_ChanllengeResult_None,
		}
	}
	now := time.Now().Unix()
	if codeInfo.ExpireTime < now {
		log.Debug("挑战码已过期: 挑战码=%d, 过期时间=%d, 当前时间=%d", req.Code, codeInfo.ExpireTime, now)
		return &message.S2C_ChanllengeByCode{
			RankItem: nil,
			Result:   message.ChanllengeResult_ChanllengeResult_CodeExpired,
		}
	}
	rankItem := r.getRankItem(codeInfo.PlayerId)
	if rankItem == nil {
		return &message.S2C_ChanllengeByCode{
			RankItem: nil,
			Result:   message.ChanllengeResult_ChanllengeResult_CodeError,
		}
	}
	return &message.S2C_ChanllengeByCode{
		RankItem: rankItem.ToMsg(),
		Result:   message.ChanllengeResult_ChanllengeResult_SUCCESS,
	}
}

// todo 整理
func (r *RankManager) getRankItem(playerId int64) *models.RankItem {
	for _, items := range r.RankCache {
		for _, rankData := range items {
			item := rankData.GetRankItem(playerId)
			if item != nil {
				return item
			}
		}
	}
	// todo 整理塞入缓存
	for _, rankType := range types {
		item := r.doHandleUpdateRankData(playerId, &message.C2S_UpdateRankData{
			RankType: rankType,
			Score:    0,
		})
		if !item.Success {
			return nil
		}
	}
	return r.getRankItem(playerId)
}

func (r *RankManager) InitMyRank(playerId int64) {
	r.SendTask(func() {
		for _, rankType := range types {
			r.doHandleUpdateRankData(playerId, &message.C2S_UpdateRankData{
				RankType: rankType,
				Score:    0,
			})
		}
	})
}

func (r *RankManager) UpdatePower(playerId int64, power float64) {
	r.SendTask(func() {
		now := time.Now()
		for _, rankType := range types {
			if rankType == message.RankType_RankType_PowerPoint {
				continue
			}
			rankData := r.GetRankData(rankType)
			// 查找是否已存在该玩家
			playerIndex := rankData.GetRankItemIndex(playerId)
			if playerIndex < 0 {
				continue
			}
			item := rankData.Items[playerIndex]
			item.UpdatePower(power)
			rankData.ItemsCache[playerId] = item
			rankData.UpdateTime = now
		}
	})
}

func (r *RankManager) GetRankData(rankType message.RankType) *models.RankData {
	seasonManager := GetSeasonManager()
	season := seasonManager.GetCurrentSeason()
	return r.GetRankDataBySeason(rankType, season)
}

func (r *RankManager) GetRankDataBySeason(rankType message.RankType, season int32) *models.RankData {
	var rankData *models.RankData
	// 赛季类型赛季需要填充，其他类型初始化的时候就已经初始化了
	if r.isSeasonType(rankType) {
		if season <= 0 {
			return nil
		}
		_, ok := r.RankCache[rankType][season]
		if !ok {
			rankData = &models.RankData{
				Season:     season,
				RankType:   rankType,
				Items:      make([]*models.RankItem, 0),
				UpdateTime: time.Now(),
			}
			r.RankCache[rankType][season] = rankData
		} else {
			rankData = r.RankCache[rankType][season]
		}
	} else {
		rankData = r.RankCache[rankType][0]
	}
	return rankData
}

// HandleGetMyRank 获取我的排名 - 异步执行
func (r *RankManager) HandleGetMyRank(playerId int64, rankType message.RankType, season int32) *message.S2C_GetMyRank {
	response := r.SendTask(func() *message.S2C_GetMyRank {
		return r.doHandleGetMyRank(playerId, rankType, season)
	})

	return response.(*message.S2C_GetMyRank)
}

// doHandleGetMyRank 获取我的排名的同步实现
func (r *RankManager) doHandleGetMyRank(playerId int64, rankType message.RankType, season int32) *message.S2C_GetMyRank {
	response := &message.S2C_GetMyRank{RankType: rankType}
	if _, ok := r.players.FindOnline(playerId); !ok {
		return response
	}
	rankData := r.GetRankDataBySeason(message.RankType(rankType), season)

	// 查找玩家排名
	for i, item := range rankData.Items {
		if item.PlayerId == playerId {
			response.MyRank = int32(i + 1)
			response.MyScore = int32(item.Score)
		}
	}
	response.TotalCount = int32(len(rankData.Items))
	return response
}

func (r *RankManager) GetMyHistoryRank(playerId int64) *message.S2C_GetMyHistoryRank {
	response := r.SendTask(func() *message.S2C_GetMyHistoryRank {
		return r.doHandleGetMyHistoryRank(playerId)
	})
	return response.(*message.S2C_GetMyHistoryRank)
}

// doHandleGetMyHistoryRank 获取我的历史排名的同步实现
func (r *RankManager) doHandleGetMyHistoryRank(playerId int64) *message.S2C_GetMyHistoryRank {
	response := &message.S2C_GetMyHistoryRank{
		Daily: r.GetHistoryRank(playerId, 1),
		Week:  r.GetHistoryRank(playerId, 2),
	}
	return response
}

func (r *RankManager) GetHistoryRankReward(playerId int64, req *message.C2S_GetHistoryRankReward) *message.S2C_GetHistoryRankReward {
	response := r.SendTask(func() *message.S2C_GetHistoryRankReward {
		return r.doHandleGetHistoryRankReward(playerId, req)
	})
	return response.(*message.S2C_GetHistoryRankReward)
}

// doHandleGetHistoryRankReward 获取历史排名奖励的同步实现
func (r *RankManager) doHandleGetHistoryRankReward(playerId int64, req *message.C2S_GetHistoryRankReward) *message.S2C_GetHistoryRankReward {
	rankItem := r.GetHistoryRank(playerId, req.Type)
	if rankItem.AcceptReward {
		return &message.S2C_GetHistoryRankReward{
			Success:  false,
			Type:     req.Type,
			RankItem: rankItem,
		}
	}
	return &message.S2C_GetHistoryRankReward{
		Success:  true,
		Type:     req.Type,
		RankItem: rankItem,
	}
}

// // 1为日榜，2为周榜
func (r *RankManager) GetHistoryRank(playerId int64, t int32) *message.HistoryRankItem {
	result := &message.HistoryRankItem{
		CurrentRank: -1,
		LastRank:    -1,
	}
	seasonManager := GetSeasonManager()
	season := seasonManager.GetCurrentSeason()
	currentRankDatas := r.GetRankDataBySeason(message.RankType_RankType_LadderPoint, season)
	nowRankItem := currentRankDatas.GetRankItem(playerId)
	if nowRankItem != nil {
		result.CurrentRank = nowRankItem.Rank
	}
	if t == 1 {
		// 获取当前日榜
		if item, ok := currentRankDatas.HistoryItemCache[playerId]; ok {
			result.LastRank = item.Rank
			result.AcceptReward = item.AcceptDaylyReward
		}
	} else if t == 2 {
		// 获取上赛季周榜
		lastRankDatas := r.GetRankDataBySeason(message.RankType_RankType_LadderPoint, season-1)
		if lastRankDatas != nil {
			if item, ok := lastRankDatas.HistoryItemCache[playerId]; ok {
				result.LastRank = item.Rank
				result.AcceptReward = item.AcceptWeeklyReward
			}
		}
	}
	return result
}

func (r *RankManager) isSeasonType(rankType message.RankType) bool {
	seasonTypes := []message.RankType{
		message.RankType_RankType_LadderPoint,
	}
	for _, t := range seasonTypes {
		if t == rankType {
			return true
		}
	}
	return false
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
		r.RankCache = make(map[message.RankType]map[int32]*models.RankData)
	} else {
		r.RankCache = data.RankCache
	}
	r.rankListCache = make(map[int64]map[message.RankType][]*models.RankItem)
	r.PersistId = 1 // 使用固定ID，因为现在使用单例模式
	log.Debug("从数据库加载排行榜数据: %v", r)
}
