package player

// todo 设置信息
type PlayerInfo struct {
	ServerId   int32  `bson:"server_id" default:"0"`
	PlayerName string `bson:"player_name" default:""`
	Avatar     string `bson:"avatar" default:""`
	Level      int32  `bson:"level" default:"0"`
	// todo 其他信息
}
