package managers

import (
	
	actor_manager "gameserver/core/actor"
	"gameserver/core/gate"
	
	"sync"
)

type MatchManagerActorProxy struct {
	// 给manager暴露出来调用不走actor队列
	DirectCaller *MatchManager
}

var (
	actorProxy *MatchManagerActorProxy
	matchManagerOnce sync.Once
)

func GetMatchManagerActorId() int64 {
	return 1
}

func GetMatchManager() *MatchManagerActorProxy {
	matchManagerOnce.Do(func() {
		matchManagerMeta, _ := actor_manager.Register[MatchManager](GetMatchManagerActorId(), actor_manager.User)
		actorProxy = &MatchManagerActorProxy{
			DirectCaller: matchManagerMeta.Actor,
		}
	})
	return actorProxy
}


// HandleMatch 调用MatchManager的HandleMatch方法
func (*MatchManagerActorProxy) HandleMatch(agent gate.Agent) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, agent)
	

	actor_manager.Send[MatchManager](GetMatchManagerActorId(), "HandleMatch", sendArgs)
}


// HandleCancelMatch 调用MatchManager的HandleCancelMatch方法
func (*MatchManagerActorProxy) HandleCancelMatch(agent gate.Agent) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, agent)
	

	actor_manager.Send[MatchManager](GetMatchManagerActorId(), "HandleCancelMatch", sendArgs)
}


