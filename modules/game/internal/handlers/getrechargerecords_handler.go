package handlers

import (
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game/internal/managers"
)

// C2S_GetRechargeRecordsHandler 处理获取充值记录请求
func C2S_GetRechargeRecordsHandler(args []interface{}) {
	if len(args) < 2 {
		log.Error("C2S_GetRechargeRecordsHandler: 参数不足")
		return
	}

	msg, ok := args[0].(*message.C2S_GetRechargeRecords)
	if !ok {
		log.Error("C2S_GetRechargeRecordsHandler: 消息类型错误")
		return
	}

	agent, ok := args[1].(gate.Agent)
	if !ok {
		log.Error("C2S_GetRechargeRecordsHandler: Agent类型错误")
		return
	}

	log.Debug("收到获取充值记录请求: %v, agent: %v", msg, agent)

	// 获取玩家ID
	userData := agent.UserData()
	if userData == nil {
		log.Error("C2S_GetRechargeRecordsHandler: 用户数据为空")
		agent.WriteMsg(&message.S2C_GetRechargeRecords{
			Records: []*message.RechargeRecord{},
		})
		return
	}

	user, ok := userData.(models.User)
	if !ok {
		log.Error("C2S_GetRechargeRecordsHandler: 用户数据类型错误")
		agent.WriteMsg(&message.S2C_GetRechargeRecords{
			Records: []*message.RechargeRecord{},
		})
		return
	}

	// 验证限制数量
	limit := int(msg.Limit)
	if limit <= 0 {
		limit = 20 // 默认返回20条记录
	} else if limit > 100 {
		limit = 100 // 最大返回100条记录
	}

	// 调用充值管理器获取记录
	rechargeManager := managers.GetRechargeManager()
	records := rechargeManager.GetPlayerRechargeRecords(user.PlayerId, limit)

	// 转换为protobuf消息
	var pbRecords []*message.RechargeRecord
	for _, record := range records {
		pbRecord := &message.RechargeRecord{
			Id:            record.Id,
			PlayerId:      record.PlayerId,
			AccountId:     record.AccountId,
			Amount:        record.Amount,
			Currency:      record.Currency,
			Platform:      int32(record.Platform),
			Status:        int32(record.Status),
			OrderId:       record.OrderId,
			TransactionId: record.TransactionId,
			CreateTime:    record.CreateTime,
			UpdateTime:    record.UpdateTime,
			CompleteTime:  record.CompleteTime,
			Description:   record.Description,
		}
		pbRecords = append(pbRecords, pbRecord)
	}

	// 发送响应给客户端
	agent.WriteMsg(&message.S2C_GetRechargeRecords{
		Records: pbRecords,
	})

	log.Debug("充值记录获取成功: PlayerId=%d, 返回 %d 条记录", user.PlayerId, len(pbRecords))
}
