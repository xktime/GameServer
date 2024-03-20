package messages

import "GameServer/server/znet/messages/proto"

type C2SMessage struct {
	MessageId proto.C2SMessageId
	Message   interface{}
}

func (r *C2SMessage) GetMessageId() uint32 {
	return uint32(r.MessageId)
}

func (r *C2SMessage) GetMessage() interface{} {
	return r.Message
}
