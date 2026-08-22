package player

import (
	"gameserver/common/msg/message"
	"strconv"
)

type PlayerInfo struct {
	PlayerName    string `bson:"player_name" default:""`
	AvatarSuffix  string `bson:"avatar_suffix" default:""`
	PlayerId      int64  `bson:"player_id" default:"0"`
	Balance       int64  `bson:"balance" default:"0"`        // 账户余额（分）
	TotalRecharge int64  `bson:"total_recharge" default:"0"` // 累计充值金额（分）
	ServerId      int32  `bson:"server_id" default:"0"`
	Level         int32  `bson:"level" default:"0"`
	VipLevel      int32  `bson:"vip_level" default:"0"` // VIP等级
}

func (p *PlayerInfo) ToMsg() *message.PlayerInfo {
	return &message.PlayerInfo{
		ServerId:   p.ServerId,
		PlayerName: p.PlayerName,
		Avatar:     p.GetAvatarURL(),
		Level:      p.Level,
		PlayerId:   strconv.FormatInt(p.PlayerId, 10),
	}
}

func (p *PlayerInfo) GetAvatarURL() string {
	if p.AvatarSuffix != "" {
		return "https://rank-server.oss-cn-hangzhou.aliyuncs.com/avatar/" + strconv.FormatInt(p.PlayerId, 10) + p.AvatarSuffix
	} else {
		return "https://file.gugudang.com/res/down/public/icon/avatar/1002058.jpg"
	}
}

// SaveData 保存数据
type SaveData struct {
	Id   string `bson:"_id"`  // playerid_type
	Data string `bson:"data"` // 数据
}

func (p SaveData) GetPersistId() interface{} {
	return p.Id
}
