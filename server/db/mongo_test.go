package db

import (
	"GameServer/server/common"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"testing"
	_ "testing"
)

// 创建连接的时候执行
func TestMongo(t *testing.T) {
	// 加载配置
	common.Load()
	// 加载数据库
	Load()

	result, _ := GetMongoClient().InsertOne("testing", "users", bson.D{{"fullName", "User 2"}, {"age", 89}})
	fmt.Println("inset:", result.InsertedID)
	findResult := GetMongoClient().FindOne("testing", "users", bson.D{{"fullName", "User 2"}})
	fmt.Println("findResult:", findResult)
}
