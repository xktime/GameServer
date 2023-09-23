package db

import (
	"fmt"
	"github.com/spf13/viper"
	"sync"
)

type DB struct {
	MongoClient MongoManager `mapstructure:"mongo"`
}

var (
	mu       sync.Mutex
	instance *DB
)

func GetDBInstance() *DB {
	if instance == nil {
		mu.Lock()
		defer mu.Unlock()
		if instance == nil {
			instance = &DB{}
		}
	}
	return instance
}

func GetMongoClient() *MongoManager {
	return &GetDBInstance().MongoClient
}

func Load() {
	manager := GetDBInstance()
	if err := viper.Unmarshal(&manager); err != nil {
		fmt.Printf("Error Unmarshal dbManager, %s", err)
	}
	// 加载mongo
	manager.MongoClient.Load()
}
