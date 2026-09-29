package managers

import (
	"context"
	"gameserver/common/msg/message"
	"gameserver/modules/rank/internal/documents"
	"gameserver/modules/rank/internal/models"
	"math"
	"testing"
)

func TestRankManagerRetainsOffBoardScoresAcrossUpdatesAndRestart(t *testing.T) {
	players := fakePlayerReader{}
	for id := int64(1); id <= 1001; id++ {
		players[id] = players[0]
	}
	repository := &memoryRankStore{}
	manager, system := startTestRankManager(t, repository, players, rankTestNow)
	rankType := message.RankType_RankType_ChallengePoint
	for id := int64(1); id <= 1001; id++ {
		if response := manager.HandleUpdateRankData(id, &message.C2S_UpdateRankData{RankType: rankType, Score: 2000 - int32(id)}); !response.Success {
			t.Fatalf("update player %d: %v", id, response)
		}
	}
	assertRank := func(manager *RankManager, id int64, rank, score int32) {
		t.Helper()
		response := manager.HandleGetMyRank(id, rankType, 0)
		if response == nil || response.MyRank != rank || response.MyScore != score || response.TotalCount != 1000 {
			t.Fatalf("player %d: got %v, want rank=%d score=%d total=1000", id, response, rank, score)
		}
	}
	assertRank(manager, 1001, 0, 999)
	manager.UpdatePower(1001, 123.45)
	manager.InitMyRank(1001)
	assertRank(manager, 1001, 0, 999)
	code := manager.GeneratorChallengeCode(&message.C2S_GeneratorChanllengeCode{PlayerId: "1001"})
	if code == nil {
		t.Fatal("off-board player cannot generate challenge code")
	}
	manager.HandleUpdateRankData(1001, &message.C2S_UpdateRankData{RankType: rankType, Score: 999})
	if response := manager.HandleGetRankList(1, &message.C2S_GetRankList{RankType: rankType, Page: 1, PageSize: 2000}); response.TotalCount != 1000 || len(response.RankItems) != 1000 {
		t.Fatalf("display must be limited to 1000: %v", response)
	}
	manager.HandleUpdateRankData(1, &message.C2S_UpdateRankData{RankType: rankType, Score: 1})
	assertRank(manager, 1, 0, 1)
	assertRank(manager, 1001, 1000, 999)
	manager.HandleUpdateRankData(1, &message.C2S_UpdateRankData{RankType: rankType, Score: 3000})
	assertRank(manager, 1, 1, 3000)
	assertRank(manager, 1001, 0, 999)
	if err := system.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	boards, err := repository.LoadBoards()
	if err != nil {
		t.Fatal(err)
	}
	for _, board := range boards {
		if board.RankType != rankType {
			continue
		}
		for _, item := range board.Items {
			if item.PlayerId == 1001 && (item.GetPower() != 123.45 || len(item.OtherInfos) != 2) {
				t.Fatalf("off-board updates lost power or challenge code: %+v", item)
			}
		}
	}
	restarted, _ := startTestRankManager(t, repository, players, rankTestNow)
	assertRank(restarted, 1001, 0, 999)
	assertRank(restarted, 1, 1, 3000)
}

func TestRankManagerReturnsIndependentResponsesAndBoundsPages(t *testing.T) {
	manager, system := startTestRankManager(t, &memoryRankStore{}, fakePlayerReader{42: {}, 43: {}}, rankTestNow)
	rankType := message.RankType_RankType_ChallengePoint
	manager.HandleUpdateRankData(42, &message.C2S_UpdateRankData{RankType: rankType, Score: 100})
	manager.HandleUpdateRankData(43, &message.C2S_UpdateRankData{RankType: rankType, Score: 50})
	manager.UpdatePower(42, 123.45)
	req := &message.C2S_GetRankList{RankType: rankType}
	page := manager.HandleGetRankList(42, req)
	if page.TotalCount != 2 || page.CurrentPage != 1 || len(page.RankItems) != 2 || req.Page != 0 || req.PageSize != 0 {
		t.Fatalf("page defaults or request ownership are wrong: %+v %+v", page, req)
	}
	page.RankItems[0].OtherInfos[0].Value = "changed by caller"
	page.RankItems[0].Score = 999
	page = manager.HandleGetRankList(42, req)
	if page.RankItems[0].OtherInfos[0].Value != "123.45" || page.RankItems[0].Score != 100 {
		t.Fatal("list response mutated owned state")
	}
	challengeReq := &message.C2S_GetChanllengeList{RankType: rankType}
	challenge := manager.GetChallengeList(43, challengeReq)
	if len(challenge.RankItem) != 1 {
		t.Fatalf("expected one opponent: %v", challenge)
	}
	challenge.RankItem[0].OtherInfos[0].Value = "changed cached response"
	if next := manager.GetChallengeList(43, challengeReq); next.RankItem[0].OtherInfos[0].Value != "123.45" {
		t.Fatal("challenge response mutated its cache")
	}
	for _, page := range []int32{2, math.MaxInt32} {
		response := manager.HandleGetRankList(42, &message.C2S_GetRankList{RankType: rankType, Page: page, PageSize: math.MaxInt32})
		if len(response.RankItems) != 0 || response.TotalCount != 2 {
			t.Fatalf("invalid page bounds: %v", response)
		}
	}
	if manager.HandleUpdateRankData(42, nil).Success || manager.HandleGetRankList(42, nil) != nil || manager.GeneratorChallengeCode(nil) != nil {
		t.Fatal("nil request was accepted")
	}
	manager.GetChallengeList(42, nil)
	manager.ChanllengeByCode(nil)
	if manager.GeneratorChallengeCode(&message.C2S_GeneratorChanllengeCode{PlayerId: "invalid"}) != nil {
		t.Fatal("invalid player ID was accepted")
	}
	if panics := system.Metrics().HandlerPanics; panics != 0 {
		t.Fatalf("request handling caused %d panics", panics)
	}
}

func TestRankManagerKeepsHistorySentinelAndBoundsSeasonQueries(t *testing.T) {
	store := &memoryRankStore{}
	rankType := message.RankType_RankType_LadderPoint
	saveTestDocument(t, store, &documents.RankManager{PersistId: 1, StorageVersion: documents.StorageVersion})
	saveTestDocument(t, store, &SeasonManager{PersistId: 1, Season: 3, LastCrossDayTime: rankTestNow().Unix()})
	board := testBoard(rankType, 3, 0)
	board.Items = nil
	for id := int64(1); id <= 1001; id++ {
		board.Items = append(board.Items, &models.RankItem{PlayerId: id, Score: 2000 - id})
	}
	saveTestDocument(t, store, board)
	saveTestDocument(t, store, testBoard(rankType, 2, 88))
	manager, system := startTestRankManager(t, store, fakePlayerReader{1001: {}, 42: {}}, rankTestNow)
	if response := manager.HandleGetMyRank(1001, rankType, 3); response.MyRank != 0 || response.MyScore != 999 {
		t.Fatalf("off-board my-rank contract changed: %v", response)
	}
	if response := manager.GetMyHistoryRank(1001); response.Daily.CurrentRank != -1 || response.Daily.LastRank != -1 {
		t.Fatalf("historical unranked sentinel changed: %v", response)
	}
	if response := manager.HandleGetRankList(42, &message.C2S_GetRankList{RankType: rankType, Season: 2}); response.Season != 2 || response.TotalCount != 1 || response.RankItems[0].Score != 88 {
		t.Fatalf("previous-season page selected wrong board: %v", response)
	}
	for _, season := range []int32{1, 4, math.MaxInt32} {
		response := manager.HandleGetRankList(42, &message.C2S_GetRankList{RankType: rankType, Season: season})
		if response.TotalCount != 0 || len(response.RankItems) != 0 {
			t.Fatalf("out-of-scope season was visible: %v", response)
		}
	}
	if err := system.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	boards, err := store.LoadBoards()
	if err != nil {
		t.Fatal(err)
	}
	for _, board := range boards {
		if board.RankType == rankType && (board.Season < 2 || board.Season > 3) {
			t.Fatalf("query created an arbitrary season: %+v", board)
		}
	}
}
