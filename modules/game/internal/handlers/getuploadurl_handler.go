package handlers

import (
	"gameserver/common/bucket"
	"gameserver/common/msg/message"
	"gameserver/common/session"
	"gameserver/core/gate"
	"gameserver/core/log"
	"strconv"
)

// C2S_GetUploadUrlHandler 处理C2S_GetUploadUrl消息
func C2S_GetUploadUrlHandler(users UserManager, args []interface{}) {
	if len(args) < 3 {
		log.Error("C2S_GetUploadUrlHandler: 参数不足")
		return
	}

	msg, ok := args[0].(*message.C2S_GetUploadUrl)
	if !ok {
		log.Error("C2S_GetUploadUrlHandler: 消息类型错误")
		return
	}

	agent, ok := args[1].(gate.Agent)
	if !ok {
		log.Error("C2S_GetUploadUrlHandler: Agent类型错误")
		return
	}

	seq, ok := args[2].(uint32)
	if !ok {
		log.Error("C2S_GetUploadUrlHandler: Seq类型错误")
		return
	}

	log.Debug("收到C2S_GetUploadUrl消息: %v, agent: %v, seq: %v", msg, agent, seq)
	playerID, ok := session.PlayerIDFrom(agent.UserData())
	if !ok {
		log.Error("C2S_GetUploadUrlHandler: Player Session 不存在")
		return
	}
	url, err := bucket.GetOSSClient().GenerateUploadUrl(msg.Type, strconv.FormatInt(playerID, 10)+"."+msg.SuffixName)
	if err != nil {
		log.Error("C2S_GetUploadUrlHandler: 生成上传URL失败: %v", err)
		return
	}
	users.ModifyAvatarSuffix(playerID, msg.SuffixName)
	resultMsg := &message.S2C_GetUploadUrl{
		Url: url,
	}
	agent.WriteMsgWithSeq(resultMsg, seq)
}
