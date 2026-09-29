package models

import (
	"gameserver/common/msg/message"
	"strconv"
	"time"
)

const (
	ChallengeCodeFloorint   = 10000
	ChallengeCodeCeilint    = 99999
	ChallengeCodeExpireTime = 365 * 24 * 60 * 60
)

type ChallengeCode struct {
	PlayerId     int64 `bson:"player_id" json:"player_id"`
	Code         int32 `bson:"code" json:"code"`
	ExpireTime   int64 `bson:"expire_time" json:"expire_time"`
	GenerateTime int64 `bson:"generate_time" json:"generate_time"`
}

// RankItem 排行榜项目
type RankItem struct {
	UpdateTime time.Time            `bson:"update_time" json:"update_time"` // 更新时间
	PlayerName string               `bson:"player_name" json:"player_name"`
	Avatar     string               `bson:"avatar" json:"avatar"`                     // 头像
	OtherInfos []*message.OtherInfo `bson:"other_infos,omitempty" json:"other_infos"` // 其他信息
	PlayerId   int64                `bson:"player_id" json:"player_id"`
	Score      int64                `bson:"score" json:"score"` // 分数/等级/战力/财富等
	Rank       int32                `bson:"rank" json:"rank"`
	Level      int32                `bson:"level" json:"level"` // 等级
}

func (r *RankItem) GetPower() float64 {
	for _, info := range r.OtherInfos {
		if info != nil && info.Key == "power" {
			power, _ := strconv.ParseFloat(info.Value, 64)
			return power
		}
	}
	return -1
}

func (r *RankItem) UpdatePower(power float64) {
	if r.OtherInfos == nil {
		r.OtherInfos = make([]*message.OtherInfo, 0)
	}
	for _, info := range r.OtherInfos {
		if info != nil && info.Key == "power" {
			info.Value = strconv.FormatFloat(power, 'f', 2, 64)
			return
		}
	}
	r.OtherInfos = append(r.OtherInfos, &message.OtherInfo{
		Key:   "power",
		Value: strconv.FormatFloat(power, 'f', 2, 64),
	})
}

func (r *RankItem) ToMsg() *message.RankItem {
	return &message.RankItem{
		PlayerId:   strconv.FormatInt(r.PlayerId, 10),
		PlayerName: r.PlayerName,
		Score:      int32(r.Score),
		Avatar:     r.Avatar,
		Level:      int32(r.Level),
		OtherInfos: cloneOtherInfos(r.OtherInfos),
		Rank:       r.Rank,
	}
}

// PlayerRankInfo 玩家排名信息
type PlayerRankInfo struct {
	UpdateTime time.Time        `bson:"update_time" json:"update_time"`
	PlayerName string           `bson:"player_name" json:"player_name"`
	PlayerId   int64            `bson:"player_id" json:"player_id"`
	Score      int64            `bson:"score" json:"score"`
	RankType   message.RankType `bson:"rank_type" json:"rank_type"`
	Rank       int32            `bson:"rank" json:"rank"`
}

type HistotyRankItem struct {
	PlayerId           int64 `bson:"player_id" json:"player_id"`
	Rank               int32 `bson:"rank" json:"rank"`
	AcceptDaylyReward  bool  `bson:"accept_dayly_reward" json:"accept_dayly_reward"`
	AcceptWeeklyReward bool  `bson:"accept_weekly_reward" json:"accept_weekly_reward"`
}
