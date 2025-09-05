package handlers

import (
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game/internal/managers"
)

// C2S_GetRechargeConfigsHandler 处理获取充值配置请求
func C2S_GetRechargeConfigsHandler(args []interface{}) {
	if len(args) < 2 {
		log.Error("C2S_GetRechargeConfigsHandler: 参数不足")
		return
	}

	agent, ok := args[1].(gate.Agent)
	if !ok {
		log.Error("C2S_GetRechargeConfigsHandler: Agent类型错误")
		return
	}

	log.Debug("收到获取充值配置请求, agent: %v", agent)

	// 调用充值管理器获取配置
	rechargeManager := managers.GetRechargeManager()
	configs := rechargeManager.GetRechargeConfigs()

	// 转换为protobuf消息
	var pbConfigs []*message.RechargeConfig
	for _, config := range configs {
		pbConfig := &message.RechargeConfig{
			Id:          config.Id,
			Name:        config.Name,
			Amount:      config.Amount,
			Bonus:       config.Bonus,
			Currency:    config.Currency,
			Description: config.Description,
			IsActive:    config.Is,
			SortOrder:   int32(config.Sort),
		}
		pbConfigs = append(pbConfigs, pbConfig)
	}

	// 发送响应给客户端
	agent.WriteMsg(&message.S2C_GetRechargeConfigs{
		Configs: pbConfigs,
	})

	log.Debug("充值配置获取成功，返回 %d 个配置", len(pbConfigs))
}
