package managers

import (
	
	
	
	
	actor_manager "gameserver/core/actor"
	
	"gameserver/common/msg/message"
	
	
	
	"sync"
)

type RankManagerActorProxy struct {
	// 给manager暴露出来调用不走actor队列
	DirectCaller *RankManager
}

var (
	rankManagerActorProxy *RankManagerActorProxy
	rankManagerOnce sync.Once
)

func GetRankManagerActorId() int64 {
	return 1
}

func GetRankManager() *RankManagerActorProxy {
	rankManagerOnce.Do(func() {
		rankManagerMeta, _ := actor_manager.Register[RankManager](GetRankManagerActorId(), actor_manager.ActorGroup("rankManager"))
		rankManagerActorProxy = &RankManagerActorProxy{
			DirectCaller: rankManagerMeta.Actor,
		}
		if actorInit, ok := interface{}(rankManagerActorProxy).(actor_manager.ActorInit); ok {
			actorInit.OnInitData()
		}
	})
	return rankManagerActorProxy
}


// OnInitData 调用RankManager的OnInitData方法
func (*RankManagerActorProxy) OnInitData() {
	sendArgs := []interface{}{}
	

	actor_manager.Send[RankManager](GetRankManagerActorId(), "OnInitData", sendArgs)
}


// HandleUpdateRankData 调用RankManager的HandleUpdateRankData方法
func (*RankManagerActorProxy) HandleUpdateRankData(playerId int64, req *message.C2S_UpdateRankData) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, playerId)
	sendArgs = append(sendArgs, req)
	

	actor_manager.Send[RankManager](GetRankManagerActorId(), "HandleUpdateRankData", sendArgs)
}


// HandleGetRankList 调用RankManager的HandleGetRankList方法
func (*RankManagerActorProxy) HandleGetRankList(playerId int64, req *message.C2S_GetRankList) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, playerId)
	sendArgs = append(sendArgs, req)
	

	actor_manager.Send[RankManager](GetRankManagerActorId(), "HandleGetRankList", sendArgs)
}


// HandleGetMyRank 调用RankManager的HandleGetMyRank方法
func (*RankManagerActorProxy) HandleGetMyRank(playerId int64, rankType int32) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, playerId)
	sendArgs = append(sendArgs, rankType)
	

	actor_manager.Send[RankManager](GetRankManagerActorId(), "HandleGetMyRank", sendArgs)
}


