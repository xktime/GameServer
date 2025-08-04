
package managers

import (
	actor_manager "gameserver/core/actor"
	"gameserver/core/gate"

	"gameserver/common/msg/message"

)


// HandleLogin 调用LoginManager的HandleLogin方法
func HandleLogin(LoginManagerId int64, args []interface{}) {
	actor_manager.Send[LoginManager](LoginManagerId, "HandleLogin", args)
}

// DoHandleLogin 调用LoginManager的DoHandleLogin方法
func DoHandleLogin(LoginManagerId int64, msg *message.C2S_Login, agent gate.Agent) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, msg)
	sendArgs = append(sendArgs, agent)

	actor_manager.Send[LoginManager](LoginManagerId, "DoHandleLogin", sendArgs)
}

// DoHandleLoginByActor 调用LoginManager的DoHandleLoginByActor方法
func DoHandleLoginByActor(LoginManagerId int64, msg *message.C2S_Login, agent gate.Agent) {
	sendArgs := []interface{}{}
	sendArgs = append(sendArgs, msg)
	sendArgs = append(sendArgs, agent)

	actor_manager.Send[LoginManager](LoginManagerId, "DoHandleLoginByActor", sendArgs)
}

