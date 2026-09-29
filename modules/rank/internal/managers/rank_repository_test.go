package managers

import (
	"context"
	"fmt"
	"gameserver/common/db/mongodb"
	"gameserver/common/msg/message"
	"gameserver/modules/rank/internal/documents"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestMongoRankRepositoryRejectsSkippedSave(t *testing.T) {
	repository := mongoRankStore{}
	if err := repository.Save(&documents.RankManager{PersistId: 1}); err == nil {
		t.Error("MongoDB 未初始化时，不应把跳过排行榜保存报告为成功")
	}
	if err := repository.Save(&SeasonManager{PersistId: 1}); err == nil {
		t.Error("MongoDB 未初始化时，不应把跳过赛季保存报告为成功")
	}
}

func TestMongoRankRepositoryMigratesAndPersistsBoards(t *testing.T) {
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		uri = "mongodb://root:1234@localhost:27017/?authSource=admin"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri).SetServerSelectionTimeout(2*time.Second))
	if err == nil {
		err = client.Ping(ctx, nil)
	}
	if err != nil {
		if client != nil {
			_ = client.Disconnect(context.Background())
		}
		if os.Getenv("MONGODB_REQUIRED") == "1" {
			t.Fatalf("MongoDB 必须可用: %v", err)
		}
		t.Skipf("MongoDB 不可用: %v", err)
	}
	databaseName := fmt.Sprintf("gameserver_rank_test_%d", time.Now().UnixNano())
	database := client.Database(databaseName)
	t.Cleanup(func() {
		cleanup, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		for _, collection := range []string{"RankManager", "SeasonManager", "Leaderboard"} {
			if err := database.Collection(collection).Drop(cleanup); err != nil {
				t.Errorf("清理 %s 测试集合: %v", collection, err)
			}
		}
		if err := mongodb.Close(cleanup); err != nil {
			t.Errorf("关闭 MongoDB: %v", err)
		}
		if err := client.Disconnect(cleanup); err != nil {
			t.Errorf("关闭测试连接: %v", err)
		}
	})
	if err := mongodb.Init(uri, databaseName, 0, 5); err != nil {
		t.Fatal(err)
	}
	// 使用旧字段布局的原始 BSON，验证真实 Mongo 的迁移与新集合写入。
	seedContext, seedCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer seedCancel()
	legacy := bson.M{"_id": int64(1), "rank_cache": bson.M{"3": bson.M{"0": bson.M{
		"rank_type": int32(3), "season": int32(0),
		"items": bson.A{bson.M{"player_id": int64(43), "score": int64(55), "update_time": rankTestNow()}},
	}}}}
	if _, err := database.Collection("RankManager").InsertOne(seedContext, legacy); err != nil {
		t.Fatal(err)
	}
	repository := mongoRankStore{}
	players := fakePlayerReader{42: {Name: "测试玩家"}, 43: {Name: "旧存档玩家"}}
	manager, system := startTestRankManager(t, repository, players, rankTestNow)
	rankType := message.RankType_RankType_ChallengePoint
	if response := manager.HandleUpdateRankData(42, &message.C2S_UpdateRankData{RankType: rankType, Score: 123}); !response.Success {
		t.Fatal("更新排行榜失败")
	}
	code := manager.GeneratorChallengeCode(&message.C2S_GeneratorChanllengeCode{PlayerId: "42"})
	if code == nil {
		t.Fatal("生成挑战码失败")
	}
	if err := system.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}

	readContext, readCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer readCancel()
	var document bson.Raw
	if err := database.Collection("RankManager").FindOne(readContext, bson.M{"_id": int64(1)}).Decode(&document); err != nil {
		t.Fatalf("排行榜集合名或 _id 不兼容: %v", err)
	}
	if index, ok := document.Lookup("code_index").Int32OK(); !ok || index != code.Code {
		t.Fatalf("code_index 字段不兼容: %v", document)
	}
	for _, field := range []string{"challenge_code_cache"} {
		if _, ok := document.Lookup(field).DocumentOK(); !ok {
			t.Fatalf("%s 字段不兼容: %v", field, document)
		}
	}
	if document.Lookup("rank_cache").Type != 0 {
		t.Fatal("new metadata must not embed leaderboards")
	}
	if version, ok := document.Lookup("storage_version").Int32OK(); !ok || version != documents.StorageVersion {
		t.Fatalf("missing storage version: %v", document)
	}
	var board documents.Leaderboard
	if err := database.Collection("Leaderboard").FindOne(readContext, bson.M{"_id": documents.BoardID(rankType, 0)}).Decode(&board); err != nil {
		t.Fatal(err)
	}
	if len(board.Items) != 2 || board.Items[0].Score != 123 || board.Items[1].PlayerId != 43 || board.Items[1].Score != 55 {
		t.Fatalf("leaderboard scores not saved separately: %+v", board)
	}
	if err := database.Collection("SeasonManager").FindOne(readContext, bson.M{"_id": int64(1)}).Decode(&document); err != nil {
		t.Fatalf("赛季集合名或 _id 不兼容: %v", err)
	}
	if _, ok := document.Lookup("season_id").Int32OK(); !ok {
		t.Fatalf("season_id 字段不兼容: %v", document)
	}

	restarted, _ := startTestRankManager(t, repository, players, rankTestNow)
	if response := restarted.HandleGetMyRank(42, rankType, 0); response == nil || response.MyScore != 123 {
		t.Fatalf("MongoDB 未恢复排名: %v", response)
	}
	if response := restarted.HandleGetMyRank(43, rankType, 0); response == nil || response.MyScore != 55 || response.MyRank != 2 {
		t.Fatalf("MongoDB 迁移未保留旧成绩: %v", response)
	}
	if response := restarted.ChanllengeByCode(&message.C2S_ChanllengeByCode{Code: code.Code}); response == nil || response.Result != message.ChanllengeResult_ChanllengeResult_SUCCESS {
		t.Fatalf("MongoDB 未恢复挑战码: %v", response)
	}
}
