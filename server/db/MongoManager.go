package db

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
)

type MongoManager struct {
	Uri    string `mapstructure:"uri"`
	client *mongo.Client
}

func (manager *MongoManager) Load() {
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(manager.Uri))
	if err != nil {
		log.Fatal(err)
	}

	// 检查连接
	err = client.Ping(context.TODO(), nil)
	if err != nil {
		log.Fatal(err)
	}
	manager.client = client
	fmt.Println("Connected to MongoDB!")
}

func (manager *MongoManager) InsertOne(db, collection string, document interface{}) *mongo.InsertOneResult {
	usersCollection := manager.client.Database(db).Collection(collection)
	result, err := usersCollection.InsertOne(context.TODO(), document)
	if err != nil {
		panic(err)
	}
	return result
}

func (manager *MongoManager) FindOne(db, collection string, filter interface{}) *mongo.Cursor {
	usersCollection := manager.client.Database(db).Collection(collection)
	result, err := usersCollection.Find(context.TODO(), filter)
	if err != nil {
		panic(err)
	}
	return result
}
