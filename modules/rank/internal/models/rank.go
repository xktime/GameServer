package models

import (
	"time"
)

// RankType 排行榜类型
type RankType int32

const (
	RankTypeLevel  RankType = iota + 1 // 等级榜
	RankTypePower                      // 战力榜
	RankTypeWealth                     // 财富榜
)

// RankItem 排行榜项目
type RankItem struct {
	PlayerId   int64     `bson:"player_id" json:"player_id"`
	PlayerName string    `bson:"player_name" json:"player_name"`
	Score      int64     `bson:"score" json:"score"`             // 分数/等级/战力/财富等
	Avatar     string    `bson:"avatar" json:"avatar"`           // 头像
	Level      int32     `bson:"level" json:"level"`             // 等级
	UpdateTime time.Time `bson:"update_time" json:"update_time"` // 更新时间
}

// RankData 排行榜数据
type RankData struct {
	RankType   RankType   `bson:"rank_type" json:"rank_type"`
	Items      []RankItem `bson:"items" json:"items"`
	UpdateTime time.Time  `bson:"update_time" json:"update_time"`
}

// PlayerRankInfo 玩家排名信息
type PlayerRankInfo struct {
	PlayerId   int64     `bson:"player_id" json:"player_id"`
	PlayerName string    `bson:"player_name" json:"player_name"`
	RankType   RankType  `bson:"rank_type" json:"rank_type"`
	Score      int64     `bson:"score" json:"score"`
	Rank       int32     `bson:"rank" json:"rank"`
	UpdateTime time.Time `bson:"update_time" json:"update_time"`
}
