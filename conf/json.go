package conf

import (
	"encoding/json"
	"gameserver/core/log"
	"os"
	"sync"
)

type JsonConf struct {
}

var (
	jsonConf     *JsonConf
	jsonConfOnce sync.Once
)

func Instance() *JsonConf {
	jsonConfOnce.Do(func() {
		jsonConf = &JsonConf{}
	})
	return jsonConf
}

var Server struct {
	LogLevel    string
	LogPath     string
	WSAddr      string
	CertFile    string
	KeyFile     string
	TCPAddr     string
	MaxConnNum  int
	ConsolePort int
	ProfilePath string
	MachineID   int64
	Debug       struct {
		Enabled bool
		Port    int
	}
	Actor struct {
		TimeoutMillisecond int
	}
	DouYinInfo struct {
		Appid     string
		Secret    string
		IsSandBox int
	}
	MongoDB struct {
		Host        string
		Database    string
		MinPoolSize uint64
		MaxPoolSize uint64
	}
}

var MongoIndexConf MongoIndexConfigs

type MongoIndexConfigs struct {
	Indexes []IndexConfig `json:"indexes"`
}

type IndexConfig struct {
	Collection string              `json:"collection"`
	Create     []IndexCreateConfig `json:"create"`
}
type IndexCreateConfig struct {
	Keys   map[string]int `json:"keys"`
	Unique bool           `json:"unique"`
}

func (j *JsonConf) Init(baseDir string) {
	// 从server.json加载Server配置
	serverPath := baseDir + "/server.json"
	data, err := os.ReadFile(serverPath)
	if err != nil {
		log.Fatal("读取server.json失败: %v", err)
	}
	err = json.Unmarshal(data, &Server)
	if err != nil {
		log.Fatal("解析server.json失败: %v", err)
	}

	// 从mongo_index.json加载Index配置
	indexPath := baseDir + "/mongo_index.json"
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		log.Fatal("读取mongo_index.json失败: %v", err)
	}

	err = json.Unmarshal(indexData, &MongoIndexConf)
	if err != nil {
		log.Fatal("解析mongo_index.json失败: %v", err)
	}
}
