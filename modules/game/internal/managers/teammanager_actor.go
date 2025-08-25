package managers

import (
	
	
	
	
	actor_manager "gameserver/core/actor"
	
	
	
	"gameserver/modules/game/internal/managers/team"
	"sync"
)

type TeamManagerActorProxy struct {
	// 给manager暴露出来调用不走actor队列
	DirectCaller *TeamManager
}

var (
	teamManageractorProxy *TeamManagerActorProxy
	teamManagerOnce sync.Once
)

func GetTeamManagerActorId() int64 {
	return 1
}

func GetTeamManager() *TeamManagerActorProxy {
	teamManagerOnce.Do(func() {
		teamManagerMeta, _ := actor_manager.Register[TeamManager](GetTeamManagerActorId(), actor_manager.ActorGroup("teamManager"))
		teamManageractorProxy = &TeamManagerActorProxy{
			DirectCaller: teamManagerMeta.Actor,
		}
		if actorInit, ok := interface{}(teamManageractorProxy).(actor_manager.ActorInit); ok {
			actorInit.OnInitData()
		}
	})
	return teamManageractorProxy
}


// GetTeamByPlayerId 调用TeamManager的GetTeamByPlayerId方法
func (*TeamManagerActorProxy) GetTeamByPlayerId(playerId int64) (*team.Team) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, playerId)
	

	future := actor_manager.RequestFuture[TeamManager](GetTeamManagerActorId(), "GetTeamByPlayerId", sendArgs)
	result, _ := future.Result()
	return result.(*team.Team)
}


// JoinRoom 调用TeamManager的JoinRoom方法
func (*TeamManagerActorProxy) JoinRoom(playerId int64, roomId int64) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, playerId)
	sendArgs = append(sendArgs, roomId)
	

	actor_manager.Send[TeamManager](GetTeamManagerActorId(), "JoinRoom", sendArgs)
}


// LeaveRoom 调用TeamManager的LeaveRoom方法
func (*TeamManagerActorProxy) LeaveRoom(teamId int64) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, teamId)
	

	actor_manager.Send[TeamManager](GetTeamManagerActorId(), "LeaveRoom", sendArgs)
}


