package managers

import (
	"context"
	"errors"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/db/mongodb"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	"gameserver/core/log"
	"gameserver/modules/rank/internal/models"
	"gameserver/modules/rank/playerread"
	"math"
	"strconv"
	"time"
)

var maxCacheSize = 1000

const managerInitializationRetryDelay = 100 * time.Millisecond

var types = []message.RankType{
	message.RankType_RankType_LadderPoint,
	message.RankType_RankType_PowerPoint,
	message.RankType_RankType_ChallengePoint,
}

// RankManager 的可变状态只在绑定的 Actor 队列中访问。
type RankManager struct {
	actor.BaseActor
	players playerread.PlayerReader
	season  *SeasonManager `bson:"-"`

	loadSeason   func() (*SeasonManager, error) `bson:"-"`
	loadRankData func(*RankManager) error       `bson:"-"`
	now          func() time.Time               `bson:"-"`
	initialized  bool                           `bson:"-"`

	// 内存缓存
	PersistId          int64                                             `bson:"_id"`
	RankCache          map[message.RankType]map[int32]*models.RankData   `bson:"rank_cache"`           // rankType -> season -> rankData
	ChallengeCodeCache map[int32]*models.ChallengeCode                   `bson:"challenge_code_cache"` // code -> challengeCode
	CodeIndex          int32                                             `bson:"code_index"`           // 挑战码索引
	rankListCache      map[int64]map[message.RankType][]*models.RankItem `bson:"-"`                    // playerId -> rankType -> rankData
}

func NewRankManager(ctx context.Context, scope *actor.Scope, players playerread.PlayerReader) (*RankManager, error) {
	return newRankManager(
		ctx,
		scope,
		players,
		newSeasonManager,
		func(manager *RankManager) error {
			return manager.loadRankDataFromDB()
		},
		time.Now,
	)
}

func newRankManager(
	ctx context.Context,
	scope *actor.Scope,
	players playerread.PlayerReader,
	loadSeason func() (*SeasonManager, error),
	loadRankData func(*RankManager) error,
	now func() time.Time,
) (*RankManager, error) {
	if players == nil {
		return nil, fmt.Errorf("rank: PlayerReader is nil")
	}
	if loadSeason == nil {
		return nil, fmt.Errorf("rank: Season loader is nil")
	}
	if loadRankData == nil {
		return nil, fmt.Errorf("rank: Rank loader is nil")
	}
	if now == nil {
		return nil, fmt.Errorf("rank: Clock is nil")
	}
	definition, err := actor.Define(scope, actor.Rank, func(context.Context, string) (*RankManager, error) {
		manager := &RankManager{
			players:            players,
			season:             &SeasonManager{PersistId: 1},
			PersistId:          1,
			RankCache:          make(map[message.RankType]map[int32]*models.RankData),
			ChallengeCodeCache: make(map[int32]*models.ChallengeCode),
			CodeIndex:          models.ChallengeCodeFloorint,
			rankListCache:      make(map[int64]map[message.RankType][]*models.RankItem),
			loadSeason:         loadSeason,
			loadRankData:       loadRankData,
			now:                now,
		}
		return manager, nil
	})
	if err != nil {
		return nil, err
	}
	manager, err := definition.GetOrCreate(ctx, "singleton")
	if err != nil {
		return nil, err
	}
	if err := manager.initializeUntilReady(ctx); err != nil {
		return nil, err
	}
	return manager, nil
}

func (m *RankManager) initializeUntilReady(ctx context.Context) error {
	for {
		err := m.initialize(ctx)
		if err == nil {
			return nil
		}
		select {
		case <-time.After(managerInitializationRetryDelay):
		case <-ctx.Done():
			return errors.Join(err, ctx.Err())
		}
	}
}

func (m *RankManager) initialize(ctx context.Context) error {
	_, err := actor.Call(ctx, m.Ref(), func(actor.Context) (struct{}, error) {
		if m.initialized {
			return struct{}{}, nil
		}
		season, err := m.loadSeason()
		if err != nil {
			return struct{}{}, err
		}
		m.season = season
		if err := m.loadRankData(m); err != nil {
			return struct{}{}, err
		}
		m.season.onCrossDay(m, m.now().Unix())
		m.initializeRankTypes()
		m.initialized = true
		return struct{}{}, nil
	})
	return err
}

func (m *RankManager) initializeRankTypes() {
	if m.RankCache == nil {
		m.RankCache = make(map[message.RankType]map[int32]*models.RankData)
	}
	season := m.season.Season
	for _, rankType := range types {
		if _, ok := m.RankCache[rankType]; ok {
			continue
		}
		m.RankCache[rankType] = make(map[int32]*models.RankData)
		rankData := &models.RankData{Season: season, RankType: rankType, Items: make([]*models.RankItem, 0), UpdateTime: time.Now()}
		if m.isSeasonType(rankType) {
			m.RankCache[rankType][season] = rankData
		} else {
			m.RankCache[rankType][0] = rankData
		}
	}
}

func (m *RankManager) OnStop(context.Context) error {
	if !m.initialized {
		return nil
	}
	_, rankErr := mongodb.Save(m)
	_, seasonErr := mongodb.Save(m.season)
	return errors.Join(rankErr, seasonErr)
}

func (m *RankManager) CheckCrossDay() {
	if err := actor.Tell(context.Background(), m.Ref(), func(actor.Context) error {
		m.season.onCrossDay(m, time.Now().Unix())
		return nil
	}); err != nil {
		log.Error("提交排行榜跨天检查失败: %v", err)
	}
}

func (m *RankManager) GetSeasonInfo() *message.S2C_SeasonInfo {
	response, err := rankCall(m, func() *message.S2C_SeasonInfo { return m.season.GetSeasonInfo() })
	if err != nil {
		log.Error("获取赛季信息失败: %v", err)
		return nil
	}
	return response
}

func rankCall[T any](manager *RankManager, task func() T) (T, error) {
	return actor.Call(context.Background(), manager.Ref(), func(actor.Context) (T, error) {
		return task(), nil
	})
}

// GetPersistId 获取持久化ID
func (r RankManager) GetPersistId() interface{} {
	return r.PersistId
}

func (r *RankManager) OnCrossDay(season int32) {
	for t, rankDatas := range r.RankCache {
		var rankData *models.RankData
		if r.isSeasonType(t) {
			rankData = rankDatas[season]
		} else {
			rankData = rankDatas[0]
		}
		if rankData != nil {
			rankData.OnCrossDay()
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

// HandleUpdateRankData 同步等待串行化的排行榜更新结果。
func (r *RankManager) HandleUpdateRankData(playerId int64, req *message.C2S_UpdateRankData) *message.S2C_UpdateRankData {
	response, err := rankCall(r, func() *message.S2C_UpdateRankData {
		return r.doHandleUpdateRankData(playerId, req)
	})
	if err != nil {
		log.Error("更新排行榜数据失败: %v", err)
		return &message.S2C_UpdateRankData{Success: false}
	}
	return response
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
			r.doUpdatePower(playerId, float64(newItem.Score))
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
	response, err := rankCall(m, func() *message.S2C_GetChanllengeList {
		return m.doHandleGetChallengeList(playerId, req)
	})
	if err != nil {
		log.Error("获取挑战列表失败: %v", err)
		return nil
	}
	return response
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

// HandleGetRankList 同步读取排行榜列表。
func (r *RankManager) HandleGetRankList(playerId int64, req *message.C2S_GetRankList) *message.S2C_GetRankList {
	response, err := rankCall(r, func() *message.S2C_GetRankList {
		return r.doHandleGetRankList(playerId, req)
	})
	if err != nil {
		log.Error("获取排行榜列表失败: %v", err)
		return nil
	}
	return response
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
	response, err := rankCall(r, func() *message.S2C_GeneratorChanllengeCode {
		return r.doGeneratorChallengeCode(req)
	})
	if err != nil {
		log.Error("生成挑战码失败: %v", err)
		return nil
	}
	return response
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
	response, err := rankCall(r, func() *message.S2C_ChanllengeByCode {
		return r.doChanllengeByCode(req)
	})
	if err != nil {
		log.Error("验证挑战码失败: %v", err)
		return nil
	}
	return response
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
	if err := actor.Tell(context.Background(), r.Ref(), func(actor.Context) error {
		for _, rankType := range types {
			r.doHandleUpdateRankData(playerId, &message.C2S_UpdateRankData{
				RankType: rankType,
				Score:    0,
			})
		}
		return nil
	}); err != nil {
		log.Error("初始化玩家排行榜失败: %v", err)
	}
}

func (r *RankManager) UpdatePower(playerId int64, power float64) {
	if err := actor.Tell(context.Background(), r.Ref(), func(actor.Context) error {
		r.doUpdatePower(playerId, power)
		return nil
	}); err != nil {
		log.Error("更新玩家战力失败: %v", err)
	}
}

func (r *RankManager) doUpdatePower(playerId int64, power float64) {
	now := time.Now()
	for _, rankType := range types {
		if rankType == message.RankType_RankType_PowerPoint {
			continue
		}
		rankData := r.GetRankData(rankType)
		playerIndex := rankData.GetRankItemIndex(playerId)
		if playerIndex < 0 {
			continue
		}
		item := rankData.Items[playerIndex]
		item.UpdatePower(power)
		rankData.ItemsCache[playerId] = item
		rankData.UpdateTime = now
	}
}

func (r *RankManager) GetRankData(rankType message.RankType) *models.RankData {
	return r.GetRankDataBySeason(rankType, r.season.Season)
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

// HandleGetMyRank 同步读取玩家排名。
func (r *RankManager) HandleGetMyRank(playerId int64, rankType message.RankType, season int32) *message.S2C_GetMyRank {
	response, err := rankCall(r, func() *message.S2C_GetMyRank {
		return r.doHandleGetMyRank(playerId, rankType, season)
	})
	if err != nil {
		log.Error("获取玩家排名失败: %v", err)
		return nil
	}
	return response
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
	response, err := rankCall(r, func() *message.S2C_GetMyHistoryRank {
		return r.doHandleGetMyHistoryRank(playerId)
	})
	if err != nil {
		log.Error("获取玩家历史排名失败: %v", err)
		return nil
	}
	return response
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
	response, err := rankCall(r, func() *message.S2C_GetHistoryRankReward {
		return r.doHandleGetHistoryRankReward(playerId, req)
	})
	if err != nil {
		log.Error("获取历史排名奖励失败: %v", err)
		return nil
	}
	return response
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
	season := r.season.Season
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
func (r *RankManager) loadRankDataFromDB() error {
	// 这里可以从数据库加载排行榜数据
	// 暂时使用空数据，实际项目中应该从数据库加载
	log.Debug("排行榜管理器初始化完成")
	data, err := mongodb.FindOneById[RankManager](r.GetPersistId())
	if err != nil {
		return fmt.Errorf("load rank data: %w", err)
	}
	if data == nil || data.RankCache == nil {
		r.RankCache = make(map[message.RankType]map[int32]*models.RankData)
	} else {
		r.RankCache = data.RankCache
	}
	r.rankListCache = make(map[int64]map[message.RankType][]*models.RankItem)
	r.PersistId = 1 // 使用固定ID，因为现在使用单例模式
	log.Debug("从数据库加载排行榜数据: %v", r)
	return nil
}
