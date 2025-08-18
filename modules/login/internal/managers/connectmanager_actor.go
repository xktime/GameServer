package managers

import (
	
	
	actor_manager "gameserver/core/actor"
	"gameserver/core/gate"
	
	
	
	"sync"
)

type ConnectManagerActorProxy struct {
	// 给manager暴露出来调用不走actor队列
	DirectCaller *ConnectManager
}

var (
	connectManageractorProxy *ConnectManagerActorProxy
	connectManagerOnce sync.Once
)

func GetConnectManagerActorId() int64 {
	return 1
}

func GetConnectManager() *ConnectManagerActorProxy {
	connectManagerOnce.Do(func() {
		connectManagerMeta, _ := actor_manager.Register[ConnectManager](GetConnectManagerActorId(), actor_manager.ActorGroup("connectManager"))
		connectManageractorProxy = &ConnectManagerActorProxy{
			DirectCaller: connectManagerMeta.Actor,
		}
		if actorInit, ok := interface{}(connectManageractorProxy).(actor_manager.ActorInit); ok {
			actorInit.OnInitData()
		}
	})
	return connectManageractorProxy
}


// OnInitData 调用ConnectManager的OnInitData方法
func (*ConnectManagerActorProxy) OnInitData() {
	sendArgs := []interface{}{}
	

	actor_manager.Send[ConnectManager](GetConnectManagerActorId(), "OnInitData", sendArgs)
}


// UpdateHeartbeat 调用ConnectManager的UpdateHeartbeat方法
func (*ConnectManagerActorProxy) UpdateHeartbeat(agent gate.Agent) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, agent)
	

	actor_manager.Send[ConnectManager](GetConnectManagerActorId(), "UpdateHeartbeat", sendArgs)
}


// RemoveClient 调用ConnectManager的RemoveClient方法
func (*ConnectManagerActorProxy) RemoveClient(clientID string) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, clientID)
	

	actor_manager.Send[ConnectManager](GetConnectManagerActorId(), "RemoveClient", sendArgs)
}


// CheckHeartbeats 调用ConnectManager的CheckHeartbeats方法
func (*ConnectManagerActorProxy) CheckHeartbeats() {
	sendArgs := []interface{}{}
	

	actor_manager.Send[ConnectManager](GetConnectManagerActorId(), "CheckHeartbeats", sendArgs)
}


// GetActiveClients 调用ConnectManager的GetActiveClients方法
func (*ConnectManagerActorProxy) GetActiveClients() (int) {
	sendArgs := []interface{}{}
	

	future := actor_manager.RequestFuture[ConnectManager](GetConnectManagerActorId(), "GetActiveClients", sendArgs)
	result, _ := future.Result()
	return result.(int)
}


// GetAllClients 调用ConnectManager的GetAllClients方法
func (*ConnectManagerActorProxy) GetAllClients() (map[string]*ClientHeartbeat) {
	sendArgs := []interface{}{}
	

	future := actor_manager.RequestFuture[ConnectManager](GetConnectManagerActorId(), "GetAllClients", sendArgs)
	result, _ := future.Result()
	return result.(map[string]*ClientHeartbeat)
}


