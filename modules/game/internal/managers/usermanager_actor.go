package managers

import (
	"gameserver/common/models"
	actor_manager "gameserver/core/actor"
	"gameserver/core/gate"
	
	"sync"
)

type UserManagerActorProxy struct {
	// 给manager暴露出来调用不走actor队列
	DirectCaller *UserManager
}

var (
	actorProxy *UserManagerActorProxy
	userManagerOnce sync.Once
)

func GetUserManagerActorId() int64 {
	return 1
}

func GetUserManager() *UserManagerActorProxy {
	userManagerOnce.Do(func() {
		userManagerMeta, _ := actor_manager.Register[UserManager](GetUserManagerActorId(), actor_manager.User)
		actorProxy = &UserManagerActorProxy{
			DirectCaller: userManagerMeta.Actor,
		}
	})
	return actorProxy
}


// UserLogin 调用UserManager的UserLogin方法
func (*UserManagerActorProxy) UserLogin(agent gate.Agent, openId string, serverId int32) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, agent)
	sendArgs = append(sendArgs, openId)
	sendArgs = append(sendArgs, serverId)
	

	actor_manager.Send[UserManager](GetUserManagerActorId(), "UserLogin", sendArgs)
}


// UserOffline 调用UserManager的UserOffline方法
func (*UserManagerActorProxy) UserOffline(user models.User) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, user)
	

	actor_manager.Send[UserManager](GetUserManagerActorId(), "UserOffline", sendArgs)
}


// GetUserByOpenId 调用UserManager的GetUserByOpenId方法
func (*UserManagerActorProxy) GetUserByOpenId(openId string, serverId int32) (models.User, bool) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, openId)
	sendArgs = append(sendArgs, serverId)
	

	future := actor_manager.RequestFuture[UserManager](GetUserManagerActorId(), "GetUserByOpenId", sendArgs)
	result, _ := future.Result()
	if resultSlice, ok := result.([]interface{}); ok && len(resultSlice) == 2 {
		ret0 := resultSlice[0].(models.User)
		ret1 := resultSlice[1].(bool)
		return ret0, ret1
	}
	return models.User{}, false
}


// GetUser 调用UserManager的GetUser方法
func (*UserManagerActorProxy) GetUser(accountId string) (models.User, bool) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, accountId)
	

	future := actor_manager.RequestFuture[UserManager](GetUserManagerActorId(), "GetUser", sendArgs)
	result, _ := future.Result()
	if resultSlice, ok := result.([]interface{}); ok && len(resultSlice) == 2 {
		ret0 := resultSlice[0].(models.User)
		ret1 := resultSlice[1].(bool)
		return ret0, ret1
	}
	return models.User{}, false
}


// GetUsers 调用UserManager的GetUsers方法
func (*UserManagerActorProxy) GetUsers() ([]models.User) {
	sendArgs := []interface{}{}
	

	future := actor_manager.RequestFuture[UserManager](GetUserManagerActorId(), "GetUsers", sendArgs)
	result, _ := future.Result()
	return result.([]models.User)
}


// GetCacheStats 调用UserManager的GetCacheStats方法
func (*UserManagerActorProxy) GetCacheStats() (map[string]interface{}) {
	sendArgs := []interface{}{}
	

	future := actor_manager.RequestFuture[UserManager](GetUserManagerActorId(), "GetCacheStats", sendArgs)
	result, _ := future.Result()
	return result.(map[string]interface{})
}


// ClearAllCache 调用UserManager的ClearAllCache方法
func (*UserManagerActorProxy) ClearAllCache() {
	sendArgs := []interface{}{}
	

	actor_manager.Send[UserManager](GetUserManagerActorId(), "ClearAllCache", sendArgs)
}


// IsUserOnline 调用UserManager的IsUserOnline方法
func (*UserManagerActorProxy) IsUserOnline(accountId string) (bool) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, accountId)
	

	future := actor_manager.RequestFuture[UserManager](GetUserManagerActorId(), "IsUserOnline", sendArgs)
	result, _ := future.Result()
	return result.(bool)
}


// GetUserLoginTime 调用UserManager的GetUserLoginTime方法
func (*UserManagerActorProxy) GetUserLoginTime(accountId string) (int64, bool) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, accountId)
	

	future := actor_manager.RequestFuture[UserManager](GetUserManagerActorId(), "GetUserLoginTime", sendArgs)
	result, _ := future.Result()
	if resultSlice, ok := result.([]interface{}); ok && len(resultSlice) == 2 {
		ret0 := resultSlice[0].(int64)
		ret1 := resultSlice[1].(bool)
		return ret0, ret1
	}
	return 0, false
}


