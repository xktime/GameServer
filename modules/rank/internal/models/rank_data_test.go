package models_test

import (
	"gameserver/common/msg/message"
	"gameserver/modules/rank/internal/documents"
	"gameserver/modules/rank/internal/models"
	"math"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func TestRankDataOwnsCopiesAndPreservesExtraInformation(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	rank := models.NewRankData(message.RankType_RankType_ChallengePoint, 0)
	input := models.RankItem{PlayerId: 42, Score: 10, UpdateTime: now,
		OtherInfos: []*message.OtherInfo{{Key: "power", Value: "123"}}}
	rank.UpdateScore(input)
	input.OtherInfos[0].Value = "bad input mutation"
	rank.UpdateScore(models.RankItem{PlayerId: 42, Score: 20, UpdateTime: now})
	item := rank.GetRankItem(42)
	if item.GetPower() != 123 {
		t.Fatalf("score update erased existing power: %v", item)
	}
	item.Score = -1
	item.OtherInfos[0].Value = "bad query mutation"
	page, total := rank.Page(1, 20)
	if total != 1 || len(page) != 1 {
		t.Fatalf("unexpected page: %v total=%d", page, total)
	}
	page[0].OtherInfos[0].Value = "bad page mutation"
	rank.OnCrossDay(now.Add(24 * time.Hour).Unix())
	snapshot := rank.Export()
	snapshot.Items[0].Score = -2
	snapshot.HistoryItemCache[42].Rank = 99
	if item := rank.GetRankItem(42); item.Score != 20 || item.GetPower() != 123 || rank.History(42).Rank != 1 {
		t.Fatalf("caller mutated owned state: %v", item)
	}
	restored := models.RestoreRankData(rank.Export())
	independent := restored.Export()
	loaded := models.RestoreRankData(independent)
	independent.Items[0].OtherInfos[0].Value = "bad restore input mutation"
	if loaded.GetRankItem(42).GetPower() != 123 {
		t.Fatal("restore retained input pointers")
	}
	msg := restored.GetRankItem(42).ToMsg()
	msg.OtherInfos[0].Value = "bad message mutation"
	if restored.GetRankItem(42).GetPower() != 123 {
		t.Fatal("message retained owned pointers")
	}
	if page, total := rank.Page(math.MaxInt32, math.MaxInt32); len(page) != 0 || total != 1 {
		t.Fatal("large page overflowed")
	}
}

func TestRankDataTieOrderSurvivesBSONAndSnapshotReplay(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	rank := models.NewRankData(message.RankType_RankType_LadderPoint, 3)
	for id := int64(1001); id > 0; id-- {
		// BSON 时间精度为毫秒，同一毫秒内按 Player ID 排序。
		rank.UpdateScore(models.RankItem{PlayerId: id, Score: 100, UpdateTime: now.Add(time.Duration(1001 - id))})
	}
	day := now.Add(24 * time.Hour).Unix()
	rank.OnCrossDay(day)
	snapshot := rank.Export()
	if len(snapshot.Items) != 1001 || len(snapshot.HistoryItemCache) != 1000 || rank.History(1001) != nil {
		t.Fatal("full scores and history display limit diverged")
	}
	snapshot.HistoryItemCache[1].AcceptDaylyReward = true
	encoded, err := bson.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var decoded models.RankSnapshot
	if err := bson.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	restored := models.RestoreRankData(decoded)
	for id := int64(1); id <= 1001; id++ {
		want := int32(id)
		if id > 1000 {
			want = 0
		}
		if item := restored.GetRankItem(id); item.Rank != want || item.Score != 100 {
			t.Fatalf("unstable tie order for %d: %+v", id, item)
		}
	}
	restored.UpdateScore(models.RankItem{PlayerId: 1, Score: 1, UpdateTime: time.Unix(day, 0)})
	restored.OnCrossDay(day + 60)
	if history := restored.History(1); history.Rank != 1 || !history.AcceptDaylyReward {
		t.Fatal("snapshot replay reset rank or reward progress")
	}
	restored.OnCrossDay(day + 24*60*60)
	if restored.History(1) != nil {
		t.Fatal("next day retained an off-board player in history")
	}
}

func capacitySnapshot() models.RankSnapshot {
	snapshot := models.RankSnapshot{RankType: message.RankType_RankType_LadderPoint, Season: 3}
	for id := int64(1); id <= 10000; id++ {
		snapshot.Items = append(snapshot.Items, &models.RankItem{
			PlayerId: id, Score: id, PlayerName: strings.Repeat("a", 18), Avatar: strings.Repeat("a", 300),
			UpdateTime: time.Unix(1800000000, 0),
			OtherInfos: []*message.OtherInfo{{Key: "power", Value: "12345.67"}, {Key: "chanllenge_key", Value: "12345"}},
		})
	}
	return snapshot
}

func TestRankDataTenThousandScoresRoundTrip(t *testing.T) {
	rank := models.RestoreRankData(capacitySnapshot())
	rank.OnCrossDay(1800100000)
	board := documents.Leaderboard{PersistId: documents.BoardID(message.RankType_RankType_LadderPoint, 3), RankSnapshot: rank.Export()}
	encoded, err := bson.Marshal(board)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) >= 16*1024*1024 {
		t.Fatalf("synthetic 10k board exceeds BSON document limit: %d", len(encoded))
	}
	var decoded documents.Leaderboard
	if err := bson.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	restored := models.RestoreRankData(decoded.RankSnapshot)
	if len(restored.Export().Items) != 10000 || restored.DisplayCount() != 1000 || restored.GetRankItem(1).Score != 1 || restored.GetRankItem(1).Rank != 0 {
		t.Fatal("10k round trip lost off-board scores")
	}
	t.Logf("10k scores with 300-byte avatars and top-1000 history: %d BSON bytes (%.3f MiB)", len(encoded), float64(len(encoded))/(1024*1024))
}

func BenchmarkRankDataUpdate10000(b *testing.B) {
	rank := models.RestoreRankData(capacitySnapshot())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		score := int64(0)
		if i%2 == 0 {
			score = int64(20000 + i%10000)
		}
		rank.UpdateScore(models.RankItem{PlayerId: int64((i*7919)%10000 + 1), Score: score, UpdateTime: time.Unix(1800000000, 0)})
	}
}
