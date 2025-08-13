package managers

import (
	
	
	actor_manager "gameserver/core/actor"
	"gameserver/core/gate"
	"gameserver/common/msg/message"
	
	
	"gameserver/common/db/mongodb"
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
		managerActor := loginManagerMeta.Actor
		if persistManager, ok := interface{}(managerActor).(mongodb.PersistManager); ok {
			persistManager.OnInitData()
		}
		loginManageractorProxy = &LoginManagerActorProxy{
			DirectCaller: managerActor,
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


