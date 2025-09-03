package managers

import (
	
	
	config "gameserver/common/config/generated"
	"gameserver/modules/game/internal/models/recharge"
	actor_manager "gameserver/core/actor"
	"gameserver/core/gate"
	"gameserver/common/msg/message"
	
	
	
	"sync"
)

type RechargeManagerActorProxy struct {
	// 给manager暴露出来调用不走actor队列
	DirectCaller *RechargeManager
}

var (
	rechargeManagerActorProxy *RechargeManagerActorProxy
	rechargeManagerOnce sync.Once
)

func GetRechargeManagerActorId() int64 {
	return 1
}

func GetRechargeManager() *RechargeManagerActorProxy {
	rechargeManagerOnce.Do(func() {
		rechargeManagerMeta, _ := actor_manager.Register[RechargeManager](GetRechargeManagerActorId(), actor_manager.ActorGroup("rechargeManager"))
		rechargeManagerActorProxy = &RechargeManagerActorProxy{
			DirectCaller: rechargeManagerMeta.Actor,
		}
		if actorInit, ok := interface{}(rechargeManagerActorProxy).(actor_manager.ActorInit); ok {
			actorInit.OnInitData()
		}
	})
	return rechargeManagerActorProxy
}


// HandleRechargeRequest 调用RechargeManager的HandleRechargeRequest方法
func (*RechargeManagerActorProxy) HandleRechargeRequest(req *RechargeRequest, agent gate.Agent) (*message.S2C_RechargeResponse) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, req)
	sendArgs = append(sendArgs, agent)
	

	future := actor_manager.RequestFuture[RechargeManager](GetRechargeManagerActorId(), "HandleRechargeRequest", sendArgs)
	result, _ := future.Result()
	return result.(*message.S2C_RechargeResponse)
}


// HandlePaymentCallback 调用RechargeManager的HandlePaymentCallback方法
func (*RechargeManagerActorProxy) HandlePaymentCallback(orderId string, transactionId string, success bool) (error) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, orderId)
	sendArgs = append(sendArgs, transactionId)
	sendArgs = append(sendArgs, success)
	

	future := actor_manager.RequestFuture[RechargeManager](GetRechargeManagerActorId(), "HandlePaymentCallback", sendArgs)
	result, _ := future.Result()
	return result.(error)
}


// GetRechargeConfigs 调用RechargeManager的GetRechargeConfigs方法
func (*RechargeManagerActorProxy) GetRechargeConfigs() ([]*config.Recharge) {
	sendArgs := []interface{}{}
	

	future := actor_manager.RequestFuture[RechargeManager](GetRechargeManagerActorId(), "GetRechargeConfigs", sendArgs)
	result, _ := future.Result()
	return result.([]*config.Recharge)
}


// GetPlayerRechargeRecords 调用RechargeManager的GetPlayerRechargeRecords方法
func (*RechargeManagerActorProxy) GetPlayerRechargeRecords(playerId int64, limit int) ([]recharge.RechargeRecord) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, playerId)
	sendArgs = append(sendArgs, limit)
	

	future := actor_manager.RequestFuture[RechargeManager](GetRechargeManagerActorId(), "GetPlayerRechargeRecords", sendArgs)
	result, _ := future.Result()
	return result.([]recharge.RechargeRecord)
}


