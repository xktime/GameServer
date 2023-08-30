package db

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoManager struct {
	Uri    string `mapstructure:"uri"`
	client *mongo.Client
}

func (manager *MongoManager) Load() {
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(manager.Uri))
	if err != nil {
		panic(err)
	}
	manager.client = client
}

func (manager *MongoManager) Insert() {
	usersCollection := manager.client.Database("testing").Collection("users")
	user := bson.D{{"fullName", "User 1"}, {"age", 70}}
	result, err := usersCollection.InsertOne(context.TODO(), user)
	if err != nil {
		panic(err)
	}
	// display the id of the newly inserted object
	fmt.Println(result.InsertedID)
}
