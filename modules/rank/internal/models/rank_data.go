package models

import (
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	"sort"
	"time"
)

const DisplayLimit = 1000

// RankSnapshot 是独立的存档副本，字段名兼容旧 RankManager 中的嵌入文档。
type RankSnapshot struct {
	Season           int32                      `bson:"season"`
	RankType         message.RankType           `bson:"rank_type"`
	Items            []*RankItem                `bson:"items,omitempty"`
	UpdateTime       time.Time                  `bson:"update_time"`
	HistoryItemCache map[int64]*HistotyRankItem `bson:"history_cache,omitempty"`
	SnapshotTime     int64                      `bson:"snapshot_time"`
}

func (s *RankSnapshot) AfterLoad() {
	if s.Items == nil {
		s.Items = make([]*RankItem, 0)
	}
	if s.HistoryItemCache == nil {
		s.HistoryItemCache = make(map[int64]*HistotyRankItem)
	}
}

// RankData 拥有完整成绩；有序成绩与索引只在本类型中修改。
// 调用者与持久化层只能获得副本，前 DisplayLimit 名之外的成绩仍保留。
type RankData struct {
	data RankSnapshot
	byID map[int64]*RankItem
}

func NewRankData(rankType message.RankType, season int32) *RankData {
	return RestoreRankData(RankSnapshot{RankType: rankType, Season: season})
}

func RestoreRankData(snapshot RankSnapshot) *RankData {
	r := &RankData{data: cloneSnapshot(snapshot), byID: make(map[int64]*RankItem)}
	r.data.AfterLoad()
	items := r.data.Items
	r.data.Items = make([]*RankItem, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		item.UpdateTime = item.UpdateTime.Truncate(time.Millisecond)
		if previous := r.byID[item.PlayerId]; previous != nil {
			*previous = *item
			continue
		}
		r.byID[item.PlayerId] = item
		r.data.Items = append(r.data.Items, item)
	}
	r.sort()
	return r
}

// UpdateScore 覆盖成绩和展示资料，保留已有战力、挑战码等扩展信息。
func (r *RankData) UpdateScore(item RankItem) {
	item.UpdateTime = item.UpdateTime.Truncate(time.Millisecond)
	if previous := r.byID[item.PlayerId]; previous != nil {
		item.OtherInfos = previous.OtherInfos
		*previous = item
	} else {
		stored := cloneRankItem(&item)
		r.byID[item.PlayerId] = stored
		r.data.Items = append(r.data.Items, stored)
	}
	r.data.UpdateTime = item.UpdateTime
	r.sort()
}

func (r *RankData) sort() {
	sort.Slice(r.data.Items, func(i, j int) bool {
		a, b := r.data.Items[i], r.data.Items[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if !a.UpdateTime.Equal(b.UpdateTime) {
			return a.UpdateTime.Before(b.UpdateTime)
		}
		return a.PlayerId < b.PlayerId
	})
	for i, item := range r.data.Items {
		item.Rank = 0
		if i < DisplayLimit {
			item.Rank = int32(i + 1)
		}
	}
}

func (r *RankData) GetRankItem(playerID int64) *RankItem {
	if r == nil {
		return nil
	}
	return cloneRankItem(r.byID[playerID])
}

func (r *RankData) DisplayCount() int32 {
	return int32(min(len(r.data.Items), DisplayLimit))
}

// Page 只读取展示范围；使用 int64 计算偏移，避免客户端页码乘法溢出。
func (r *RankData) Page(page, size int32) ([]*RankItem, int32) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	total := r.DisplayCount()
	start := int64(page-1) * int64(size)
	result := make([]*RankItem, 0)
	if start >= int64(total) {
		return result, total
	}
	end := min(start+int64(size), int64(total))
	for _, item := range r.data.Items[start:end] {
		result = append(result, cloneRankItem(item))
	}
	return result, total
}

func (r *RankData) SetOtherInfo(playerID int64, key, value string, now time.Time) {
	item := r.byID[playerID]
	if item == nil {
		return
	}
	r.data.UpdateTime = now
	for _, info := range item.OtherInfos {
		if info != nil && info.Key == key {
			info.Value = value
			return
		}
	}
	item.OtherInfos = append(item.OtherInfos, &message.OtherInfo{Key: key, Value: value})
}

// OnCrossDay 按单榜进度去重，防止部分保存后重启重复生成快照。
func (r *RankData) OnCrossDay(now int64) {
	if now <= r.data.SnapshotTime || !utils.IsCrossDay(r.data.SnapshotTime, now) {
		return
	}
	r.data.HistoryItemCache = make(map[int64]*HistotyRankItem)
	for _, item := range r.data.Items[:r.DisplayCount()] {
		r.data.HistoryItemCache[item.PlayerId] = &HistotyRankItem{PlayerId: item.PlayerId, Rank: item.Rank}
	}
	r.data.SnapshotTime = now
}

func (r *RankData) History(playerID int64) *HistotyRankItem {
	if r == nil || r.data.HistoryItemCache[playerID] == nil {
		return nil
	}
	item := *r.data.HistoryItemCache[playerID]
	return &item
}

func (r *RankData) Export() RankSnapshot {
	return cloneSnapshot(r.data)
}

func cloneSnapshot(source RankSnapshot) RankSnapshot {
	result := source
	result.Items = make([]*RankItem, 0, len(source.Items))
	for _, item := range source.Items {
		result.Items = append(result.Items, cloneRankItem(item))
	}
	result.HistoryItemCache = make(map[int64]*HistotyRankItem, len(source.HistoryItemCache))
	for id, item := range source.HistoryItemCache {
		if item != nil {
			copy := *item
			result.HistoryItemCache[id] = &copy
		}
	}
	return result
}

func cloneRankItem(item *RankItem) *RankItem {
	if item == nil {
		return nil
	}
	copy := *item
	copy.OtherInfos = cloneOtherInfos(item.OtherInfos)
	return &copy
}

func cloneOtherInfos(infos []*message.OtherInfo) []*message.OtherInfo {
	result := make([]*message.OtherInfo, 0, len(infos))
	for _, info := range infos {
		if info != nil {
			result = append(result, &message.OtherInfo{Key: info.Key, Value: info.Value})
		}
	}
	return result
}
