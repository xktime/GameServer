
package team

import (
	actor_manager "gameserver/core/actor"
	
	
	
	
)


// JoinTeam 调用Team的JoinTeam方法
func JoinTeam(TeamId int64, playerId int64) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, playerId)
	
	actor_manager.Send[Team](TeamId, "JoinTeam", sendArgs)
}

// LeaveTeam 调用Team的LeaveTeam方法
func LeaveTeam(TeamId int64, playerId int64) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, playerId)
	
	actor_manager.Send[Team](TeamId, "LeaveTeam", sendArgs)
}

// IsMember 调用Team的IsMember方法
func IsMember(TeamId int64, playerId int64) (bool) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, playerId)
	
	future := actor_manager.RequestFuture[Team](TeamId, "IsMember", sendArgs)
	result, _ := future.Result()
	return result.(bool)
}

// IsLeader 调用Team的IsLeader方法
func IsLeader(TeamId int64, playerId int64) (bool) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, playerId)
	
	future := actor_manager.RequestFuture[Team](TeamId, "IsLeader", sendArgs)
	result, _ := future.Result()
	return result.(bool)
}

