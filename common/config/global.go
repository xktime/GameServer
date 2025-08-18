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
		globalManager.LoadAllConfigs()
	})
}

// LoadConfig 全局加载配置文件
func LoadConfig(filename string) error {
	return globalManager.LoadConfig(filename)
}

// GetConfig 全局获取配置
func GetConfig(filename, id string) (interface{}, bool) {
	return globalManager.GetConfig(filename, id)
}

// GetConfigByID 全局根据ID获取配置
func GetConfigByID(id string) (string, interface{}, bool) {
	return globalManager.GetConfigByID(id)
}

// GetAllConfigs 全局获取所有配置
func GetAllConfigs(filename string) (map[string]interface{}, bool) {
	return globalManager.GetAllConfigs(filename)
}

// ReloadConfig 全局重新加载配置
func ReloadConfig(filename string) error {
	return globalManager.ReloadConfig(filename)
}

// ReloadAll 全局重新加载所有配置
func ReloadAll() error {
	return globalManager.ReloadAll()
}

// ListLoadedFiles 全局列出已加载文件
func ListLoadedFiles() []string {
	return globalManager.ListLoadedFiles()
}

// LoadAllConfigs 全局加载所有配置
func LoadAllConfigs() error {
	return globalManager.LoadAllConfigs()
}
