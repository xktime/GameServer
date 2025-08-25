package handlers

import (
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game/internal/managers"
)

// C2S_RechargeRequestHandler 处理充值请求
func C2S_RechargeRequestHandler(args []interface{}) {
	if len(args) < 2 {
		log.Error("C2S_RechargeRequestHandler: 参数不足")
		return
	}

	msg, ok := args[0].(*message.C2S_RechargeRequest)
	if !ok {
		log.Error("C2S_RechargeRequestHandler: 消息类型错误")
		return
	}

	agent, ok := args[1].(gate.Agent)
	if !ok {
		log.Error("C2S_RechargeRequestHandler: Agent类型错误")
		return
	}

	log.Debug("收到充值请求: %v, agent: %v", msg, agent)

	// 获取玩家ID
	userData := agent.UserData()
	if userData == nil {
		log.Error("C2S_RechargeRequestHandler: 用户数据为空")
		agent.WriteMsg(&message.S2C_RechargeResponse{
			Success: false,
			Message: "用户未登录",
		})
		return
	}

	user, ok := userData.(models.User)
	if !ok {
		log.Error("C2S_RechargeRequestHandler: 用户数据类型错误")
		agent.WriteMsg(&message.S2C_RechargeResponse{
			Success: false,
			Message: "用户数据错误",
		})
		return
	}

	// 验证充值金额
	if msg.Amount <= 0 {
		log.Error("C2S_RechargeRequestHandler: 充值金额无效: %d", msg.Amount)
		agent.WriteMsg(&message.S2C_RechargeResponse{
			Success: false,
			Message: "充值金额必须大于0",
		})
		return
	}

	// 验证支付平台
	if msg.Platform < 1 || msg.Platform > 3 {
		log.Error("C2S_RechargeRequestHandler: 支付平台无效: %d", msg.Platform)
		agent.WriteMsg(&message.S2C_RechargeResponse{
			Success: false,
			Message: "不支持的支付平台",
		})
		return
	}

	// 构建充值请求
	rechargeReq := &managers.RechargeRequest{
		PlayerId:  user.PlayerId,
		AccountId: user.AccountId,
		Amount:    msg.Amount,
		Platform:  msg.Platform,
		ConfigId:  msg.ConfigId,
	}

	// 调用充值管理器处理
	rechargeManager := managers.GetRechargeManager()
	response := rechargeManager.HandleRechargeRequest(rechargeReq, agent)
	agent.WriteMsg(response)
	if response.Success {
		log.Debug("充值请求处理成功: PlayerId=%d, Amount=%d, OrderId=%s",
			user.PlayerId, msg.Amount, response.OrderId)
	} else {
		log.Error("充值请求处理失败: PlayerId=%d, Amount=%d, Error=%s",
			user.PlayerId, msg.Amount, response.Message)
	}
}
