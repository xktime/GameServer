package participant

import "google.golang.org/protobuf/proto"

// PlayerMessenger 尽力向在线真人玩家投递消息，不保证送达。
type PlayerMessenger interface {
	Send(playerIDs []int64, msg proto.Message)
	SendExcept(playerIDs []int64, excludedPlayerID int64, msg proto.Message)
}
