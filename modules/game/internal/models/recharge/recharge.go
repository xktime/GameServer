package recharge

import (
	"gameserver/common/msg/message"
	"time"

	"github.com/google/uuid"
)

// 充值状态
type RechargeStatus int32

const (
	RechargeStatus_Pending   RechargeStatus = 0 // 待处理
	RechargeStatus_Success   RechargeStatus = 1 // 成功
	RechargeStatus_Failed    RechargeStatus = 2 // 失败
	RechargeStatus_Cancelled RechargeStatus = 3 // 已取消
)

// 充值记录
type RechargeRecord struct {
	Id            string                  `bson:"_id"`            // 充值订单ID
	PlayerId      int64                   `bson:"player_id"`      // 玩家ID
	AccountId     string                  `bson:"account_id"`     // 账户ID
	Amount        int64                   `bson:"amount"`         // 充值金额（分）
	Currency      string                  `bson:"currency"`       // 货币类型（CNY）
	Platform      message.PaymentPlatform `bson:"platform"`       // 支付平台
	Status        RechargeStatus          `bson:"status"`         // 充值状态
	ConfigId      string                  `bson:"config_id"`      // 充值配置ID
	OrderId       string                  `bson:"order_id"`       // 第三方订单ID
	TransactionId string                  `bson:"transaction_id"` // 交易流水号
	CreateTime    int64                   `bson:"create_time"`    // 创建时间
	UpdateTime    int64                   `bson:"update_time"`    // 更新时间
	CompleteTime  int64                   `bson:"complete_time"`  // 完成时间
	Description   string                  `bson:"description"`    // 充值描述
	Extra         map[string]interface{}  `bson:"extra"`          // 扩展字段
}

// 获取持久化ID
func (r RechargeRecord) GetPersistId() interface{} {
	return r.Id
}

// 创建新的充值记录
func NewRechargeRecord(playerId int64, accountId string, amount int64, platform message.PaymentPlatform, configId string) *RechargeRecord {
	now := time.Now().Unix()
	return &RechargeRecord{
		Id:          generateOrderId(),
		PlayerId:    playerId,
		AccountId:   accountId,
		Amount:      amount,
		Currency:    "CNY",
		Platform:    platform,
		Status:      RechargeStatus_Pending,
		ConfigId:    configId,
		CreateTime:  now,
		UpdateTime:  now,
		Description: "游戏充值",
		Extra:       make(map[string]interface{}),
	}
}

// 生成订单ID
func generateOrderId() string {
	return uuid.New().String()
}
