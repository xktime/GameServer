package test

import (
	"context"
	"fmt"
	"gameserver/common/db/mongodb"
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
