package messages

import "GameServer/server/znet/messages/proto"

type C2SMessage struct {
	MessageId proto.C2SMessageId
	Message   ProtoMessage
}

func (r *C2SMessage) GetMessageId() uint32 {
	return uint32(r.MessageId)
}

func (r *C2SMessage) GetProtoMessage() ProtoMessage {
	return r.Message
}
