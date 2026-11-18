package managers

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	"testing"
	"time"
)

func TestRankRecoveryRetriesInitializationAndLoadsPersistedState(t *testing.T) {
	store := &memoryRankStore{}
	manager, _ := startTestRankManager(t, store, fakePlayerReader{42: {Name: "player"}}, rankTestNow)
	manager.HandleUpdateRankData(42, &message.C2S_UpdateRankData{RankType: message.RankType_RankType_PowerPoint, Score: 100})
	previous := manager.currentActor()
	if _, err := actor.Command(context.Background(), previous.Ref(), func(execution actor.Context) (struct{}, error) {
		return struct{}{}, previous.OnStop(execution)
	}); err != nil {
		t.Fatal(err)
	}
	manager.HandleUpdateRankData(42, &message.C2S_UpdateRankData{RankType: message.RankType_RankType_PowerPoint, Score: 200})
	for range 3 {
		_, err := actor.Command(context.Background(), previous.Ref(), func(actor.Context) (struct{}, error) { panic("quarantine") })
		if !errors.Is(err, actor.ErrHandlerPanic) {
			t.Fatal(err)
		}
	}
	store.loadRankError = errors.New("storage unavailable")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := manager.Recover(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("failed initialization = %v", err)
	}
	if manager.currentActor() != previous {
		t.Fatal("recovery published a partially initialized actor")
	}
	store.loadRankError = nil
	if err := manager.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	response := manager.HandleGetMyRank(42, message.RankType_RankType_PowerPoint, 0)
	if response == nil || response.MyScore != 100 || manager.currentActor() == previous {
		t.Fatalf("persisted rank was not restored: %#v", response)
	}
}
