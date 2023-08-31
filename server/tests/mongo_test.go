package tests

import (
	"GameServer/server/common"
	"GameServer/server/db"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"testing"
	_ "testing"
)

// 创建连接的时候执行
func TestMongo(t *testing.T) {
	// todo 加载整理
	// 加载配置
	common.Load()
	// 加载数据库
	db.Load()

	result := db.GetDBInstance().MongoClient.InsertOne("testing", "users", bson.D{{"fullName", "User 1"}, {"age", 89}})
	fmt.Println("inset:", result.InsertedID)
	findResult := db.GetDBInstance().MongoClient.FindOne("testing", "users", bson.D{{"fullName", "User 1"}})
	fmt.Println("findResult:", findResult)
}
