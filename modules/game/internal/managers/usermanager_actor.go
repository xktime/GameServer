
package managers

import (
	actor_manager "gameserver/core/actor"
	"gameserver/core/gate"
	"gameserver/common/models"

)


// DoLoginByActor 调用UserManager的DoLoginByActor方法
func DoLoginByActor(UserManagerId int64, agent gate.Agent, openId string, serverId int32) {
	args := []interface{}{}
	args = append(args, agent)
	args = append(args, openId)
	args = append(args, serverId)

	actor_manager.Send[UserManager](UserManagerId, "DoLoginByActor", args)
}

// DoLogin 调用UserManager的DoLogin方法
func DoLogin(UserManagerId int64, agent gate.Agent, openId string, serverId int32) {
	args := []interface{}{}
	args = append(args, agent)
	args = append(args, openId)
	args = append(args, serverId)

	actor_manager.Send[UserManager](UserManagerId, "DoLogin", args)
}

// SetUserCache 调用UserManager的SetUserCache方法
func SetUserCache(UserManagerId int64) {
	args := []interface{}{}

	actor_manager.Send[UserManager](UserManagerId, "SetUserCache", args)
}

// GetCache 调用UserManager的GetCache方法
func GetCache(UserManagerId int64, accountId string) (models.User, bool) {
	args := []interface{}{}
	args = append(args, accountId)

	future := actor_manager.RequestFuture[UserManager](UserManagerId, "GetCache", args)
	result, _ := future.Result()
	if resultSlice, ok := result.([]interface{}); ok && len(resultSlice) == 2 {
		ret0 := resultSlice[0].(models.User)
		ret1 := resultSlice[1].(bool)
		return ret0, ret1
	}
	return models.User{}, false
}

// GenToken 调用UserManager的GenToken方法
func GenToken(UserManagerId int64) (string) {
	args := []interface{}{}

	future := actor_manager.RequestFuture[UserManager](UserManagerId, "GenToken", args)
	result, _ := future.Result()
	return result.(string)
}

