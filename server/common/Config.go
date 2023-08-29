package common

import (
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
}
