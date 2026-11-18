package managers

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	"gameserver/modules/rank/playerread"
	"testing"
)

type failingRankPlayerReader struct{ err error }

func (r *failingRankPlayerReader) FindOnline(context.Context, int64) (playerread.PlayerSnapshot, bool, error) {
	return playerread.PlayerSnapshot{Name: "player"}, r.err == nil, r.err
}

func TestRankManagerPropagatesLookupFailuresWithoutChangingRankState(t *testing.T) {
	for _, failure := range []error{context.Canceled, context.DeadlineExceeded, actor.ErrActorQuarantined, errors.New("storage unavailable")} {
		t.Run(failure.Error(), func(t *testing.T) {
			players := &failingRankPlayerReader{}
			manager, _ := startTestRankManager(t, &memoryRankStore{}, players, rankTestNow)
			rankType := message.RankType_RankType_ChallengePoint
			if response := manager.HandleUpdateRankData(42, &message.C2S_UpdateRankData{RankType: rankType, Score: 100}); !response.Success {
				t.Fatal("seed rank failed")
			}
			players.err = failure
			r := manager.currentActor()
			operations := map[string]func(actor.Context) error{
				"update": func(ctx actor.Context) error {
					_, err := r.doHandleUpdateRankData(ctx, 42, &message.C2S_UpdateRankData{RankType: rankType, Score: 200})
					return err
				},
				"list": func(ctx actor.Context) error {
					_, err := r.doHandleGetRankList(ctx, 42, &message.C2S_GetRankList{RankType: rankType})
					return err
				},
				"my rank": func(ctx actor.Context) error {
					_, err := r.doHandleGetMyRank(ctx, 42, rankType, 0)
					return err
				},
				"challenge code": func(ctx actor.Context) error {
					_, err := r.doGeneratorChallengeCode(ctx, &message.C2S_GeneratorChanllengeCode{PlayerId: "42"})
					return err
				},
				"initialize": func(ctx actor.Context) error { return r.ensurePlayerRanks(ctx, 43) },
			}
			for name, operation := range operations {
				_, err := actor.Command(context.Background(), r.Ref(), func(ctx actor.Context) (struct{}, error) { return struct{}{}, operation(ctx) })
				if !errors.Is(err, failure) {
					t.Errorf("%s lost lookup failure: %v", name, err)
				}
			}
			_, err := actor.Query(context.Background(), r.Ref(), func(actor.Context) (struct{}, error) {
				board := r.findRankData(rankType, 0)
				if board.GetRankItem(42).Score != 100 || board.GetRankItem(43) != nil || len(r.challengeCodeCache) != 0 {
					return struct{}{}, errors.New("failed lookup changed rank state")
				}
				return struct{}{}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
