package managers

import (
	"context"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	"gameserver/modules/rank/internal/models"
	"testing"
)

func TestRankRepositorySavesDetachedActorSnapshot(t *testing.T) {
	manager, _ := startTestRankManager(t, &memoryRankStore{}, fakePlayerReader{42: {Name: "snapshot-player"}}, rankTestNow)
	rankType := message.RankType_RankType_ChallengePoint
	if response := manager.HandleUpdateRankData(42, &message.C2S_UpdateRankData{RankType: rankType, Score: 100}); !response.Success {
		t.Fatal("seed rank failed")
	}
	code := manager.GeneratorChallengeCode(&message.C2S_GeneratorChanllengeCode{PlayerId: "42"})
	if code == nil {
		t.Fatal("seed challenge code failed")
	}
	season := manager.GetSeasonInfo().Season
	r := manager.currentActor()
	state, err := actor.Query(context.Background(), r.Ref(), func(actor.Context) (*rankState, error) {
		return r.persistenceSnapshot(), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = actor.Command(context.Background(), r.Ref(), func(actor.Context) (struct{}, error) {
		r.season.Season++
		r.codeIndex++
		r.challengeCodeCache[code.Code].ExpireTime++
		r.ensureCurrentRankData(rankType).UpdateScore(models.RankItem{PlayerId: 42, PlayerName: "changed", Score: 200, UpdateTime: rankTestNow()})
		r.ensureCurrentRankData(rankType).SetOtherInfo(42, "power", "999", rankTestNow())
		return struct{}{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryRankStore{}
	repository := &rankRepository{store: store}
	if err := repository.Save(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	persisted, err := repository.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if persisted.metadata.CodeIndex != code.Code || persisted.metadata.ChallengeCodeCache[code.Code].ExpireTime != int64(code.ExpireTimestamp) {
		t.Fatalf("live changes leaked into persisted metadata: %+v", persisted.metadata)
	}
	if persisted.season.Season != season {
		t.Fatal("live changes leaked into persisted season")
	}
	for _, board := range persisted.boards {
		if board.RankType != rankType {
			continue
		}
		if len(board.Items) != 1 || board.Items[0].Score != 100 || board.Items[0].PlayerName != "snapshot-player" {
			t.Fatalf("live changes leaked into persisted leaderboard: %+v", board.Items)
		}
		for _, info := range board.Items[0].OtherInfos {
			if info.Key == "power" && info.Value == "999" {
				t.Fatal("live changes leaked into nested leaderboard metadata")
			}
		}
		return
	}
	t.Fatal("snapshot lost the leaderboard")
}
