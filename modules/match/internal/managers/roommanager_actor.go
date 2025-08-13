package managers

import (
	
	
	actor_manager "gameserver/core/actor"
	"gameserver/core/gate"
	"gameserver/common/msg/message"
	
	
	"gameserver/common/db/mongodb"
	"sync"
)

type RoomManagerActorProxy struct {
	// 给manager暴露出来调用不走actor队列
	DirectCaller *RoomManager
}

var (
	roomManageractorProxy *RoomManagerActorProxy
	roomManagerOnce sync.Once
)

func GetRoomManagerActorId() int64 {
	return 1
}

func GetRoomManager() *RoomManagerActorProxy {
	roomManagerOnce.Do(func() {
		roomManagerMeta, _ := actor_manager.Register[RoomManager](GetRoomManagerActorId(), actor_manager.ActorGroup("roomManager"))
		managerActor := roomManagerMeta.Actor
		if persistManager, ok := interface{}(managerActor).(mongodb.PersistManager); ok {
			persistManager.OnInitData()
		}
		roomManageractorProxy = &RoomManagerActorProxy{
			DirectCaller: managerActor,
		}
	})
	return roomManageractorProxy
}


// HandleRecordOperate 调用RoomManager的HandleRecordOperate方法
func (*RoomManagerActorProxy) HandleRecordOperate(msg *message.C2S_RecordGameOperate, agent gate.Agent) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, msg)
	sendArgs = append(sendArgs, agent)
	

	actor_manager.Send[RoomManager](GetRoomManagerActorId(), "HandleRecordOperate", sendArgs)
}


