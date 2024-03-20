package messages

import "GameServer/server/znet/messages/proto"

type S2CMessage struct {
	MessageId proto.S2CMessageId
	Message   interface{}
}

func (r *S2CMessage) GetMessageId() uint32 {
	return uint32(r.MessageId)
}

func (r *S2CMessage) GetMessage() interface{} {
	return r.Message
}
