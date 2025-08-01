package config

import (
	"sync"
)

var (
	globalManager *ConfigManager
	once          sync.Once
)

// InitGlobalConfig 初始化全局配置管理器
func InitGlobalConfig(baseDir string) {
	once.Do(func() {
		globalManager = NewConfigManager(baseDir)
	})
}

// GetGlobalManager 获取全局配置管理器
func GetGlobalManager() *ConfigManager {
	if globalManager == nil {
		panic("全局配置管理器未初始化，请先调用 InitGlobalConfig")
	}
	return globalManager
}

// LoadConfig 全局加载配置文件
func LoadConfig(filename string) error {
	return GetGlobalManager().LoadConfig(filename)
}

// GetConfig 全局获取配置
func GetConfig(filename, id string) (interface{}, bool) {
	return GetGlobalManager().GetConfig(filename, id)
}

// GetConfigByID 全局根据ID获取配置
func GetConfigByID(id string) (string, interface{}, bool) {
	return GetGlobalManager().GetConfigByID(id)
}

// GetAllConfigs 全局获取所有配置
func GetAllConfigs(filename string) (map[string]interface{}, bool) {
	return GetGlobalManager().GetAllConfigs(filename)
}

// ReloadConfig 全局重新加载配置
func ReloadConfig(filename string) error {
	return GetGlobalManager().ReloadConfig(filename)
}

// ReloadAll 全局重新加载所有配置
func ReloadAll() error {
	return GetGlobalManager().ReloadAll()
}

// ListLoadedFiles 全局列出已加载文件
func ListLoadedFiles() []string {
	return GetGlobalManager().ListLoadedFiles()
}

// LoadAllConfigs 全局加载所有配置
func LoadAllConfigs() error {
	return GetGlobalManager().LoadAllConfigs()
}


// todo 需要整理到自己模块里面，模板生成结构体
// GetItemConfig 获取物品配置的便捷方法
func GetItemConfig(itemID string) (map[string]interface{}, bool) {
	config, exists := GetConfig("items.json", itemID)
	if !exists {
		return nil, false
	}

	if itemConfig, ok := config.(map[string]interface{}); ok {
		return itemConfig, true
	}

	return nil, false
}

// GetMonsterConfig 获取怪物配置的便捷方法
func GetMonsterConfig(monsterID string) (map[string]interface{}, bool) {
	config, exists := GetConfig("monsters.json", monsterID)
	if !exists {
		return nil, false
	}

	if monsterConfig, ok := config.(map[string]interface{}); ok {
		return monsterConfig, true
	}

	return nil, false
}

// GetItemName 获取物品名称
func GetItemName(itemID string) (string, bool) {
	if itemConfig, exists := GetItemConfig(itemID); exists {
		if name, ok := itemConfig["name"].(string); ok {
			return name, true
		}
	}
	return "", false
}

// GetMonsterName 获取怪物名称
func GetMonsterName(monsterID string) (string, bool) {
	if monsterConfig, exists := GetMonsterConfig(monsterID); exists {
		if name, ok := monsterConfig["name"].(string); ok {
			return name, true
		}
	}
	return "", false
}
