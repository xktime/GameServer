package test

import (
	"context"
	"fmt"
	"gameserver/common/db/mongodb"
	"gameserver/common/models"
	"gameserver/core/log"
	"testing"

	"github.com/go-redis/redis"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// 普通测试
func TestDB_TestConnect(t *testing.T) {
	ctx := context.Background()

	// 连接本地 Redis
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer rdb.Close()

	err := rdb.Set("key", "value", 0).Err()
	if err != nil {
		panic(err)
	}
	val, err := rdb.Get("key").Result()
	if err != nil {
		panic(err)
	}
	fmt.Println("redis value:", val)

	// 连接本地 MongoDB
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		panic(err)
	}

	defer mongoClient.Disconnect(ctx)

	collection := mongoClient.Database("testdb").Collection("testcol")
	doc := map[string]string{"hello": "world"}
	insertResult, err := collection.InsertOne(ctx, doc)
	if err != nil {
		panic(err)
	}
	fmt.Println("mongo insert id:", insertResult.InsertedID)
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
	mongodb.Init("mongodb://localhost:27017", "testdb", 10, 100)
	// 查询单个
	mongodb.Save(&User{ID: "1", Name: "张三", Age: 20})
	user, _ := mongodb.FindOneById[User]("1")
	fmt.Println("mongo FindOne id:", user)
	mongodb.Save(&User{ID: "2", Name: "李四", Age: 20})
	users, _ := mongodb.FindAll[User](bson.M{})
	fmt.Println("mongo InsertOne id:", users)
	// 删除
	mongodb.DeleteByID[User]("2")
	users, _ = mongodb.FindAll[User](bson.M{})
	fmt.Println("mongo DeleteByID id:", users)
	mongodb.Save(&User{ID: "1", Name: "张三123", Age: 21})
	user, _ = mongodb.FindOneById[User]("1")
	fmt.Println("mongo UpsertByID id:", user)
}

// TestBulkSave 测试批量保存功能
func TestBulkSave(t *testing.T) {
	mongodb.Init("mongodb://localhost:27017", "testMongo", 50, 50)
	// 初始化测试数据
	users := []mongodb.PersistData{
		&models.User{AccountId: "test1", ServerId: 2, OpenId: "open1", PlayerId: 1001, Platform: models.DouYin},
		&models.User{AccountId: "test2", ServerId: 2, OpenId: "open2", PlayerId: 1002, Platform: models.WeChat},
		&models.User{AccountId: "test3", ServerId: 1, OpenId: "open3", PlayerId: 1003, Platform: models.DouYin},
		&models.User{AccountId: "test5", ServerId: 5, OpenId: "open3", PlayerId: 1003, Platform: models.DouYin},
	}

	// 测试批量保存
	_, err := mongodb.BulkSave(users)
	if err != nil {
		log.Fatal("批量保存失败: %v", err)
	}

	// 验证保存结果
	for _, data := range users {
		user := data.(*models.User)
		savedUser, err := mongodb.FindOneById[models.User](user.AccountId)
		if err != nil {
			log.Fatal("查询用户失败: %v", err)
		}
		if savedUser == nil {
			log.Fatal("用户不存在: %s", user.AccountId)
		}
		if savedUser.PlayerId != user.PlayerId || savedUser.Platform != user.Platform {
			log.Fatal("用户数据不匹配: 期望PlayerId=%d, 实际PlayerId=%d", user.PlayerId, savedUser.PlayerId)
		}
	}

	log.Release("批量保存测试成功")
}
