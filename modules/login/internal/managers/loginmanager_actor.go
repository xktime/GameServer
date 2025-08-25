package managers

import (
	
	
	
	
	actor_manager "gameserver/core/actor"
	"gameserver/core/gate"
	"gameserver/common/msg/message"
	
	
	"sync"
)

type LoginManagerActorProxy struct {
	// 给manager暴露出来调用不走actor队列
	DirectCaller *LoginManager
}

var (
	loginManageractorProxy *LoginManagerActorProxy
	loginManagerOnce sync.Once
)

func GetLoginManagerActorId() int64 {
	return 1
}

func GetLoginManager() *LoginManagerActorProxy {
	loginManagerOnce.Do(func() {
		loginManagerMeta, _ := actor_manager.Register[LoginManager](GetLoginManagerActorId(), actor_manager.ActorGroup("loginManager"))
		loginManageractorProxy = &LoginManagerActorProxy{
			DirectCaller: loginManagerMeta.Actor,
		}
		if actorInit, ok := interface{}(loginManageractorProxy).(actor_manager.ActorInit); ok {
			actorInit.OnInitData()
		}
	})
	return loginManageractorProxy
}


// HandleLogin 调用LoginManager的HandleLogin方法
func (*LoginManagerActorProxy) HandleLogin(msg *message.C2S_Login, agent gate.Agent) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, msg)
	sendArgs = append(sendArgs, agent)
	

	actor_manager.Send[LoginManager](GetLoginManagerActorId(), "HandleLogin", sendArgs)
}


