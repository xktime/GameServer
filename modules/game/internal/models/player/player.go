package player

import "gameserver/common/msg/message"

// todo 设置信息
type PlayerInfo struct {
	ServerId      int32  `bson:"server_id" default:"0"`
	PlayerName    string `bson:"player_name" default:""`
	Avatar        string `bson:"avatar" default:""`
	Level         int32  `bson:"level" default:"0"`
	Balance       int64  `bson:"balance" default:"0"`        // 账户余额（分）
	TotalRecharge int64  `bson:"total_recharge" default:"0"` // 累计充值金额（分）
	VipLevel      int32  `bson:"vip_level" default:"0"`      // VIP等级
	// todo 其他信息
}

func (p *PlayerInfo) ToMsgPlayerInfo() *message.PlayerInfo {
	return &message.PlayerInfo{
		ServerId:   int32(p.ServerId),
		PlayerName: p.PlayerName,
		Avatar:     p.Avatar,
		Level:      int64(p.Level),
	}
}
