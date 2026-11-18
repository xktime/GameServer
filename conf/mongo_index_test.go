package conf

import (
	"encoding/json"
	"os"
	"testing"
)

func TestMongoIndexesSupportPlayerNameLookup(t *testing.T) {
	data, err := os.ReadFile("mongo_index.json")
	if err != nil {
		t.Fatal(err)
	}
	var configs MongoIndexConfigs
	if err := json.Unmarshal(data, &configs); err != nil {
		t.Fatal(err)
	}
	for _, config := range configs.Indexes {
		if config.Collection != "Player" {
			continue
		}
		for _, index := range config.Create {
			if len(index.Keys) == 1 && index.Keys["player_info.player_name"] == 1 && !index.Unique {
				return
			}
		}
	}
	t.Fatal("Player name lookup requires a non-unique ascending index on player_info.player_name")
}
