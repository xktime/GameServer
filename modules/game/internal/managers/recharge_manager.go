package managers

import (
	"fmt"
	"gameserver/common/base/actor"
	config "gameserver/common/config/generated"
	"gameserver/common/db/mongodb"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game/internal/managers/player"
	"gameserver/modules/game/internal/models/recharge"
	"sort"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// RechargeManager 使用TaskHandler实现，确保充值操作按顺序执行
type RechargeManager struct {
	*actor.TaskHandler
}

var (
	rechargeManager     *RechargeManager
	rechargeManagerOnce sync.Once
)

func GetRechargeManager() *RechargeManager {
	rechargeManagerOnce.Do(func() {
		rechargeManager = &RechargeManager{}
		rechargeManager.Init()
	})
	return rechargeManager
}

// Init 初始化RechargeManager
func (m *RechargeManager) Init() {
	// 初始化TaskHandler
	m.TaskHandler = actor.InitTaskHandler(actor.Recharge, "1", m)
	m.TaskHandler.Start()
}

// Stop 停止RechargeManager
func (m *RechargeManager) Stop() {
	m.TaskHandler.Stop()
}

// 全局缓存
var (
	rechargeConfigCache sync.Map // 充值配置缓存
	rechargeRecordCache sync.Map // 充值记录缓存
)

// 充值请求
type RechargeRequest struct {
	PlayerId  int64                   `json:"player_id"`
	AccountId string                  `json:"account_id"` // 账户ID
	Amount    int64                   `json:"amount"`     // 充值金额（分）
	Platform  message.PaymentPlatform `json:"platform"`   // 支付平台
	ConfigId  string                  `json:"config_id"`  // 充值配置ID（可选）
}

// HandleRechargeRequest 处理充值请求 - 异步执行
func (m *RechargeManager) HandleRechargeRequest(req *RechargeRequest, agent gate.Agent) *message.S2C_RechargeResponse {
	response := m.SendTask(func() *actor.Response {
		result := m.doHandleRechargeRequest(req, agent)
		return &actor.Response{
			Result: []interface{}{result},
		}
	})

	if response != nil && len(response.Result) > 0 {
		if result, ok := response.Result[0].(*message.S2C_RechargeResponse); ok {
			return result
		}
	}
	return &message.S2C_RechargeResponse{
		Success: false,
		Message: "处理充值请求失败",
	}
}

// doHandleRechargeRequest 处理充值请求的同步实现
func (m *RechargeManager) doHandleRechargeRequest(req *RechargeRequest, agent gate.Agent) *message.S2C_RechargeResponse {
	// 1. 验证玩家信息
	playerInstance := GetUserManager().GetPlayer(req.PlayerId)
	if playerInstance == nil {
		return &message.S2C_RechargeResponse{
			Success: false,
			Message: "玩家不存在或未在线",
		}
	}

	// 2. 验证充值金额
	if req.Amount <= 0 {
		return &message.S2C_RechargeResponse{
			Success: false,
			Message: "充值金额必须大于0",
		}
	}

	// 3. 创建充值记录
	rechargeRecord := recharge.NewRechargeRecord(
		req.PlayerId,
		req.AccountId,
		req.Amount,
		req.Platform,
		req.ConfigId,
	)

	// 4. 保存充值记录到数据库
	if _, err := mongodb.Save(rechargeRecord); err != nil {
		log.Error("保存充值记录失败: %v", err)
		return &message.S2C_RechargeResponse{
			Success: false,
			Message: "创建充值订单失败",
		}
	}

	// 5. 更新缓存
	m.updateRechargeRecordCache(rechargeRecord)

	// 6. 生成支付信息
	paymentInfo := m.generatePaymentInfo(rechargeRecord)

	log.Debug("充值请求处理成功: PlayerId=%d, Amount=%d, OrderId=%s",
		req.PlayerId, req.Amount, rechargeRecord.Id)

	return &message.S2C_RechargeResponse{
		Success:    true,
		OrderId:    rechargeRecord.Id,
		Message:    "充值请求已创建",
		PaymentUrl: paymentInfo.PaymentUrl,
		QrCode:     paymentInfo.QRCode,
	}
}

// HandlePaymentCallback 处理支付回调 - 异步执行
func (m *RechargeManager) HandlePaymentCallback(orderId, transactionId string, success bool) error {
	response := m.SendTask(func() *actor.Response {
		err := m.doHandlePaymentCallback(orderId, transactionId, success)
		return &actor.Response{
			Result: []interface{}{err},
		}
	})

	if response != nil && len(response.Result) > 0 {
		if err, ok := response.Result[0].(error); ok {
			return err
		}
	}
	return nil
}

// doHandlePaymentCallback 处理支付回调的同步实现
func (m *RechargeManager) doHandlePaymentCallback(orderId, transactionId string, success bool) error {
	// 1. 查找充值记录
	rechargeRecord := m.getRechargeRecord(orderId)
	if rechargeRecord == nil {
		log.Error("支付回调：充值记录不存在: %s", orderId)
		return nil
	}

	// 2. 检查订单状态
	if rechargeRecord.Status != recharge.RechargeStatus_Pending {
		log.Error("支付回调：订单状态异常: %s, Status: %d", orderId, rechargeRecord.Status)
		return nil
	}

	// 3. 更新订单状态
	now := time.Now().Unix()
	if success {
		rechargeRecord.Status = recharge.RechargeStatus_Success
		rechargeRecord.TransactionId = transactionId
		rechargeRecord.CompleteTime = now

		// 4. 更新玩家数据
		if err := m.updatePlayerBalance(rechargeRecord); err != nil {
			log.Error("更新玩家余额失败: %v", err)
			return err
		}
	} else {
		rechargeRecord.Status = recharge.RechargeStatus_Failed
	}

	rechargeRecord.UpdateTime = now

	// 5. 保存更新后的记录
	if _, err := mongodb.Save(rechargeRecord); err != nil {
		log.Error("保存充值记录失败: %v", err)
		return err
	}

	// 6. 更新缓存
	m.updateRechargeRecordCache(rechargeRecord)

	log.Debug("支付回调处理完成: OrderId=%s, Success=%v", orderId, success)
	return nil
}

// 更新玩家余额
func (m *RechargeManager) updatePlayerBalance(rechargeRecord *recharge.RechargeRecord) error {
	// 1. 获取玩家实例
	playerInstance := GetUserManager().GetPlayer(rechargeRecord.PlayerId)
	if playerInstance == nil {
		return fmt.Errorf("玩家不在线: %d", rechargeRecord.PlayerId)
	}

	// 2. 计算充值金额（包含赠送）
	totalAmount := rechargeRecord.Amount
	if config := m.getRechargeConfig(rechargeRecord.ConfigId); config != nil {
		totalAmount += int64(config.Bonus)
	}

	// 3. 更新玩家数据
	playerInstance.PlayerInfo.Balance += totalAmount
	playerInstance.PlayerInfo.TotalRecharge += rechargeRecord.Amount

	// 4. 计算VIP等级
	m.updateVipLevel(playerInstance)

	// 5. 保存到数据库
	if _, err := mongodb.Save(playerInstance); err != nil {
		return err
	}

	// 6. 发送充值成功通知（简化版本，直接发送文本消息）
	log.Debug("玩家充值成功: PlayerId=%d, Amount=%d, TotalAmount=%d, Balance=%d",
		rechargeRecord.PlayerId, rechargeRecord.Amount, totalAmount, playerInstance.PlayerInfo.Balance)

	return nil
}

// 更新VIP等级
func (m *RechargeManager) updateVipLevel(playerInstance *player.Player) {
	totalRecharge := playerInstance.PlayerInfo.TotalRecharge

	// todo 使用配置表
	// 简单的VIP等级计算逻辑
	var newVipLevel int32
	switch {
	case totalRecharge >= 1000000: // 10000元
		newVipLevel = 5
	case totalRecharge >= 500000: // 5000元
		newVipLevel = 4
	case totalRecharge >= 200000: // 2000元
		newVipLevel = 3
	case totalRecharge >= 100000: // 1000元
		newVipLevel = 2
	case totalRecharge >= 50000: // 500元
		newVipLevel = 1
	default:
		newVipLevel = 0
	}

	if newVipLevel > playerInstance.PlayerInfo.VipLevel {
		playerInstance.PlayerInfo.VipLevel = newVipLevel
		log.Debug("玩家VIP等级提升: PlayerId=%d, VipLevel=%d",
			playerInstance.PlayerId, newVipLevel)
	}
}

// 生成支付信息
func (m *RechargeManager) generatePaymentInfo(rechargeRecord *recharge.RechargeRecord) *PaymentInfo {
	// todo 这里应该调用真实的支付接口
	// todo 目前返回模拟数据
	return &PaymentInfo{
		PaymentUrl: "https://payment.example.com/pay/" + rechargeRecord.Id,
		QRCode:     "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==",
	}
}

// 获取充值配置
func (m *RechargeManager) getRechargeConfig(configId string) *config.Recharge {
	if configId == "" {
		return nil
	}

	// 从缓存获取
	if value, exists := rechargeConfigCache.Load(configId); exists {
		if config, ok := value.(*config.Recharge); ok {
			return config
		}
	}

	// 从配置系统获取
	config, exists := config.GetRechargeConfig(configId)
	if !exists {
		log.Error("获取充值配置失败: %s", configId)
		return nil
	}

	// 更新缓存
	m.updateRechargeConfigCache(config)

	return config
}

// 获取充值记录
func (m *RechargeManager) getRechargeRecord(orderId string) *recharge.RechargeRecord {
	// 从缓存获取
	if value, exists := rechargeRecordCache.Load(orderId); exists {
		if record, ok := value.(*recharge.RechargeRecord); ok {
			return record
		}
	}

	// 从数据库获取
	record, err := mongodb.FindOneById[recharge.RechargeRecord](orderId)
	if err != nil {
		log.Error("获取充值记录失败: %v", err)
		return nil
	}

	if record != nil {
		m.updateRechargeRecordCache(record)
	}

	return record
}

// 更新充值配置缓存
func (m *RechargeManager) updateRechargeConfigCache(config *config.Recharge) {
	rechargeConfigCache.Store(config.Id, config)
}

// 更新充值记录缓存
func (m *RechargeManager) updateRechargeRecordCache(record *recharge.RechargeRecord) {
	rechargeRecordCache.Store(record.Id, record)
}

// GetRechargeConfigs 获取充值配置列表 - 异步执行
func (m *RechargeManager) GetRechargeConfigs() []*config.Recharge {
	response := m.SendTask(func() *actor.Response {
		configs := m.doGetRechargeConfigs()
		return &actor.Response{
			Result: []interface{}{configs},
		}
	})

	if response != nil && len(response.Result) > 0 {
		if configs, ok := response.Result[0].([]*config.Recharge); ok {
			return configs
		}
	}
	return nil
}

// doGetRechargeConfigs 获取充值配置列表的同步实现
func (m *RechargeManager) doGetRechargeConfigs() []*config.Recharge {
	// 从配置系统获取所有配置
	configs, exists := config.GetAllRechargeConfigs()
	if !exists {
		log.Error("获取充值配置列表失败")
		return nil
	}

	// 转换为切片
	var configList []*config.Recharge
	for _, config := range configs {
		// 只返回激活的配置
		if config.Is {
			configList = append(configList, config)
		}
	}

	// 按排序字段排序
	sort.Slice(configList, func(i, j int) bool {
		return configList[i].Sort < configList[j].Sort
	})

	// 更新缓存
	for _, config := range configList {
		m.updateRechargeConfigCache(config)
	}

	return configList
}

// GetPlayerRechargeRecords 获取玩家充值记录 - 异步执行
func (m *RechargeManager) GetPlayerRechargeRecords(playerId int64, limit int) []recharge.RechargeRecord {
	response := m.SendTask(func() *actor.Response {
		records := m.doGetPlayerRechargeRecords(playerId, limit)
		return &actor.Response{
			Result: []interface{}{records},
		}
	})

	if response != nil && len(response.Result) > 0 {
		if records, ok := response.Result[0].([]recharge.RechargeRecord); ok {
			return records
		}
	}
	return nil
}

// doGetPlayerRechargeRecords 获取玩家充值记录的同步实现
func (m *RechargeManager) doGetPlayerRechargeRecords(playerId int64, limit int) []recharge.RechargeRecord {
	query := bson.M{"player_id": playerId}
	recordsResult, err := mongodb.FindAll[recharge.RechargeRecord](query)
	if err != nil {
		log.Error("获取玩家充值记录失败: %v", err)
		return nil
	}

	// 限制数量
	if limit > 0 && len(recordsResult) > limit {
		recordsResult = recordsResult[:limit]
	}

	return recordsResult
}

// 支付信息
type PaymentInfo struct {
	PaymentUrl string `json:"payment_url"`
	QRCode     string `json:"qr_code"`
}
