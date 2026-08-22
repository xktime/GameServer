package test

import (
	"context"
	"fmt"
	"gameserver/common/db/mongodb"
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const defaultTestMongoURI = "mongodb://root:1234@localhost:27017/?authSource=admin"

func initMongoTest(t *testing.T) {
	t.Helper()

	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		mongoURI = defaultTestMongoURI
	}
	databaseName := fmt.Sprintf("gameserver_test_%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().
		ApplyURI(mongoURI).
		SetServerSelectionTimeout(2*time.Second))
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

	if err := mongodb.Init(mongoURI, databaseName, 0, 5); err != nil {
		t.Fatalf("初始化 MongoDB 测试库失败: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		database := client.Database(databaseName)
		collectionNames, err := database.ListCollectionNames(cleanupCtx, bson.M{})
		if err != nil {
			t.Errorf("列出 MongoDB 测试集合失败: %v", err)
		} else {
			for _, collectionName := range collectionNames {
				if err := database.Collection(collectionName).Drop(cleanupCtx); err != nil {
					t.Errorf("清理 MongoDB 测试集合 %s 失败: %v", collectionName, err)
				}
			}
		}
		if err := mongodb.Close(cleanupCtx); err != nil {
			t.Errorf("断开 MongoDB 全局测试连接失败: %v", err)
		}
		if err := client.Disconnect(cleanupCtx); err != nil {
			t.Errorf("断开 MongoDB 测试连接失败: %v", err)
		}
	})
}

type User struct {
	ID   string `bson:"_id"`
	Name string `bson:"name"`
	Age  int    `bson:"age"`
}

func (u User) GetPersistId() interface{} {
	return u.ID
}

func TestDB_TestMongo(t *testing.T) {
	initMongoTest(t)

	if _, err := mongodb.Save(&User{ID: "1", Name: "张三", Age: 20}); err != nil {
		t.Fatalf("保存用户失败: %v", err)
	}
	user, err := mongodb.FindOneById[User]("1")
	if err != nil {
		t.Fatalf("查询用户失败: %v", err)
	}
	if user == nil || user.Name != "张三" || user.Age != 20 {
		t.Fatalf("查询结果不符合预期: %#v", user)
	}

	if _, err := mongodb.Save(&User{ID: "2", Name: "李四", Age: 20}); err != nil {
		t.Fatalf("保存第二个用户失败: %v", err)
	}
	users, err := mongodb.FindAll[User](bson.M{})
	if err != nil {
		t.Fatalf("查询全部用户失败: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("用户数量 = %d，期望 2", len(users))
	}

	if _, err := mongodb.DeleteByID[User]("2"); err != nil {
		t.Fatalf("删除用户失败: %v", err)
	}
	deletedUser, err := mongodb.FindOneById[User]("2")
	if err != nil {
		t.Fatalf("查询已删除用户失败: %v", err)
	}
	if deletedUser != nil {
		t.Fatalf("用户删除后仍可查询: %#v", deletedUser)
	}

	if _, err := mongodb.Save(&User{ID: "1", Name: "张三123", Age: 21}); err != nil {
		t.Fatalf("更新用户失败: %v", err)
	}
	user, err = mongodb.FindOneById[User]("1")
	if err != nil {
		t.Fatalf("查询更新后的用户失败: %v", err)
	}
	if user == nil || user.Name != "张三123" || user.Age != 21 {
		t.Fatalf("更新结果不符合预期: %#v", user)
	}
}

// TestBulkSave 测试批量保存功能
func TestBulkSave(t *testing.T) {
	initMongoTest(t)

	// 初始化测试数据
	users := []mongodb.PersistData{
		&models.User{AccountId: "test1", ServerId: 2, OpenId: "open1", PlayerId: 1001, Platform: message.LoginType_DouYin},
		&models.User{AccountId: "test2", ServerId: 2, OpenId: "open2", PlayerId: 1002, Platform: message.LoginType_WeChat},
		&models.User{AccountId: "test3", ServerId: 1, OpenId: "open3", PlayerId: 1003, Platform: message.LoginType_DouYin},
		&models.User{AccountId: "test5", ServerId: 5, OpenId: "open3", PlayerId: 1003, Platform: message.LoginType_DouYin},
	}

	// 测试批量保存
	result, err := mongodb.BulkSave(users)
	if err != nil {
		t.Fatalf("批量保存失败: %v", err)
	}
	if result == nil || result.UpsertedCount != int64(len(users)) {
		t.Fatalf("批量保存结果不符合预期: %#v", result)
	}

	// 验证保存结果
	for _, data := range users {
		user := data.(*models.User)
		savedUser, err := mongodb.FindOneById[models.User](user.AccountId)
		if err != nil {
			t.Fatalf("查询用户失败: %v", err)
		}
		if savedUser == nil {
			t.Fatalf("用户不存在: %s", user.AccountId)
		}
		if savedUser.PlayerId != user.PlayerId || savedUser.Platform != user.Platform {
			t.Fatalf("用户数据不匹配: 期望PlayerId=%d, 实际PlayerId=%d", user.PlayerId, savedUser.PlayerId)
		}
	}
}

type M struct {
	ID   string            `bson:"_id"`
	Name map[string]string `bson:"name"`
	Age  int               `bson:"age"`
}

func (m M) AddName(name string) {
	m.Name[name] = name
}

func (m M) ChangeAge(age int) {
	m.Age = age
}

func TestM(t *testing.T) {
	m := M{
		ID:   "1",
		Name: make(map[string]string),
		Age:  18,
	}

	m.AddName("123")
	m.ChangeAge(1)
	fmt.Println(m)
}
