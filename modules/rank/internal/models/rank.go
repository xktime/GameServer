package models

import (
	"gameserver/common/msg/message"
	"sort"
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
	PlayerId   int64                `bson:"player_id" json:"player_id"`
	Rank       int32                `bson:"rank" json:"rank"`
	PlayerName string               `bson:"player_name" json:"player_name"`
	Score      int64                `bson:"score" json:"score"`             // 分数/等级/战力/财富等
	Avatar     string               `bson:"avatar" json:"avatar"`           // 头像
	Level      int32                `bson:"level" json:"level"`             // 等级
	UpdateTime time.Time            `bson:"update_time" json:"update_time"` // 更新时间
	OtherInfos []*message.OtherInfo `bson:"other_infos" json:"other_infos"` // 其他信息
}

func (r *RankItem) GetPower() float64 {
	for _, info := range r.OtherInfos {
		if info.Key == "power" {
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
		if info.Key == "power" {
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
		OtherInfos: r.OtherInfos,
		Rank:       r.Rank,
	}
}

// RankData 排行榜数据
type RankData struct {
	Season           int32                      `bson:"season" json:"season"`
	RankType         message.RankType           `bson:"rank_type" json:"rank_type"`
	Items            []*RankItem                `bson:"items" json:"items"`
	ItemsCache       map[int64]*RankItem        `bson:"-" json:"-"`
	UpdateTime       time.Time                  `bson:"update_time" json:"update_time"`
	HistoryItemCache map[int64]*HistotyRankItem `bson:"history_cache" json:"history_cache"`
}

func (r *RankData) OnCrossDay() {
	r.HistoryItemCache = make(map[int64]*HistotyRankItem, 0)
	r.SortRankData()
	for _, item := range r.Items {
		r.HistoryItemCache[item.PlayerId] = &HistotyRankItem{
			PlayerId:           item.PlayerId,
			Rank:               item.Rank,
			AcceptDaylyReward:  false,
			AcceptWeeklyReward: false,
		}
	}
}

func (r *RankData) GetRankItem(playerId int64) *RankItem {
	if item, ok := r.ItemsCache[playerId]; ok {
		return item
	}
	return nil
}

func (r *RankData) GetRankItemIndex(playerId int64) int32 {
	rankItem := r.GetRankItem(playerId)
	if rankItem == nil {
		return -1
	}
	index := rankItem.Rank - 1
	if index < 0 || index >= int32(len(r.Items)) {
		return -1
	}
	if r.Items[index].PlayerId != playerId {
		return -1
	}
	return index
}

// sortRankData 对排行榜数据进行排序
func (r *RankData) SortRankData() {
	sort.Slice(r.Items, func(i, j int) bool {
		// 根据排行榜类型进行不同的排序
		switch r.RankType {
		default:
			// 分数高的排在前面
			if r.Items[i].Score != r.Items[j].Score {
				return r.Items[i].Score > r.Items[j].Score
			}
			// 分数相同时，按更新时间排序（老的排在前面）
			return r.Items[j].UpdateTime.After(r.Items[i].UpdateTime)
		}
	})
	r.ItemsCache = make(map[int64]*RankItem)
	for index, item := range r.Items {
		item.Rank = int32(index + 1)
		r.ItemsCache[item.PlayerId] = item
	}
}

// PlayerRankInfo 玩家排名信息
type PlayerRankInfo struct {
	PlayerId   int64            `bson:"player_id" json:"player_id"`
	PlayerName string           `bson:"player_name" json:"player_name"`
	RankType   message.RankType `bson:"rank_type" json:"rank_type"`
	Score      int64            `bson:"score" json:"score"`
	Rank       int32            `bson:"rank" json:"rank"`
	UpdateTime time.Time        `bson:"update_time" json:"update_time"`
}

type HistotyRankItem struct {
	PlayerId           int64 `bson:"player_id" json:"player_id"`
	Rank               int32 `bson:"rank" json:"rank"`
	AcceptDaylyReward  bool  `bson:"accept_dayly_reward" json:"accept_dayly_reward"`
	AcceptWeeklyReward bool  `bson:"accept_weekly_reward" json:"accept_weekly_reward"`
}
