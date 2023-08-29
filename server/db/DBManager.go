package db

import (
	"fmt"
	"github.com/spf13/viper"
)

type DBManager struct {
	MongoClient MongoManager `mapstructure:"mongo"`
}

func (manager *DBManager) Load() {
	if err := viper.Unmarshal(&manager); err != nil {
		fmt.Printf("Error Unmarshal dbManager, %s", err)
	}
	manager.MongoClient.Load()
}
