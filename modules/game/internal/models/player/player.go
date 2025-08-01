package player

type PlayerInfo struct {
	PlayerId       int64   `bson:"_id"`
	ServerId        int32    `bson:"ServerId" default:"0"`
	// todo 其他信息
}