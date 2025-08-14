package managers

import (
	
	
	actor_manager "gameserver/core/actor"
	"gameserver/core/gate"
	"gameserver/common/msg/message"
	
	
	"sync"
)

type MatchManagerActorProxy struct {
	// 给manager暴露出来调用不走actor队列
	DirectCaller *MatchManager
}

var (
	matchManageractorProxy *MatchManagerActorProxy
	matchManagerOnce sync.Once
)

func GetMatchManagerActorId() int64 {
	return 1
}

func GetMatchManager() *MatchManagerActorProxy {
	matchManagerOnce.Do(func() {
		matchManagerMeta, _ := actor_manager.Register[MatchManager](GetMatchManagerActorId(), actor_manager.ActorGroup("matchManager"))
		matchManageractorProxy = &MatchManagerActorProxy{
			DirectCaller: matchManagerMeta.Actor,
		}
		if actorInit, ok := interface{}(matchManageractorProxy).(actor_manager.ActorInit); ok {
			actorInit.OnInitData()
		}
	})
	return matchManageractorProxy
}


// OnInitData 调用MatchManager的OnInitData方法
func (*MatchManagerActorProxy) OnInitData() {
	sendArgs := []interface{}{}
	

	actor_manager.Send[MatchManager](GetMatchManagerActorId(), "OnInitData", sendArgs)
}


// Matching 调用MatchManager的Matching方法
func (*MatchManagerActorProxy) Matching() {
	sendArgs := []interface{}{}
	

	actor_manager.Send[MatchManager](GetMatchManagerActorId(), "Matching", sendArgs)
}


// HandleMatch 调用MatchManager的HandleMatch方法
func (*MatchManagerActorProxy) HandleMatch(agent gate.Agent, msg *message.C2S_StartMatch) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, agent)
	sendArgs = append(sendArgs, msg)
	

	actor_manager.Send[MatchManager](GetMatchManagerActorId(), "HandleMatch", sendArgs)
}


// HandleCancelMatch 调用MatchManager的HandleCancelMatch方法
func (*MatchManagerActorProxy) HandleCancelMatch(agent gate.Agent) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, agent)
	

	actor_manager.Send[MatchManager](GetMatchManagerActorId(), "HandleCancelMatch", sendArgs)
}


// ProcessTimeoutRequests 调用MatchManager的ProcessTimeoutRequests方法
func (*MatchManagerActorProxy) ProcessTimeoutRequests() {
	sendArgs := []interface{}{}
	

	actor_manager.Send[MatchManager](GetMatchManagerActorId(), "ProcessTimeoutRequests", sendArgs)
}


