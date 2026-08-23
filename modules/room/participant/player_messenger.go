package participant

import "google.golang.org/protobuf/proto"

// PlayerMessenger delivers best-effort messages to online real Players.
type PlayerMessenger interface {
	Send(playerIDs []int64, msg proto.Message)
	SendExcept(playerIDs []int64, excludedPlayerID int64, msg proto.Message)
}
