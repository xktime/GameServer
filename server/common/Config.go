package common

import (
	"GameServer/server/db"
	"fmt"
	"github.com/spf13/viper"
)

type Config struct {
}

func (n *Config) Load() {
	viper.AddConfigPath("$.")
	viper.AddConfigPath("conf")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("Error reading config file, %s", err)
	}
	var dbManager db.DB
	if err := viper.Unmarshal(&dbManager); err != nil {
		fmt.Printf("Error Unmarshal dbManager, %s", err)
	}

}
