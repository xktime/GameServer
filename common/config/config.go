package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
)

// ConfigManager 配置管理器
type ConfigManager struct {
	configs map[string]map[string]interface{} // 文件名 -> {ID -> 配置数据}
	mu      sync.RWMutex
	baseDir string
}

// NewConfigManager 创建新的配置管理器
func NewConfigManager(baseDir string) *ConfigManager {
	return &ConfigManager{
		configs: make(map[string]map[string]interface{}),
		baseDir: baseDir,
	}
}

// LoadConfig 加载指定JSON配置文件
func (cm *ConfigManager) LoadConfig(filename string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	filePath := filepath.Join(cm.baseDir, filename)

	// 读取文件
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败 %s: %v", filePath, err)
	}

	// 解析JSON数组
	var configArray []map[string]interface{}
	if err := json.Unmarshal(data, &configArray); err != nil {
		return fmt.Errorf("解析JSON失败 %s: %v", filePath, err)
	}

	// 创建ID到配置的映射
	configMap := make(map[string]interface{})
	for _, item := range configArray {
		if id, ok := item["id"]; ok {
			idStr := fmt.Sprintf("%v", id)
			configMap[idStr] = item
		}
	}

	cm.configs[filename] = configMap
	return nil
}

// GetConfig 根据文件名和ID获取配置
func (cm *ConfigManager) GetConfig(filename, id string) (interface{}, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	configMap, exists := cm.configs[filename]
	if !exists {
		return nil, false
	}

	config, exists := configMap[id]
	return config, exists
}

// GetConfigByID 根据ID获取配置（自动查找所有已加载的文件）
func (cm *ConfigManager) GetConfigByID(id string) (string, interface{}, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for filename, configMap := range cm.configs {
		if config, exists := configMap[id]; exists {
			return filename, config, true
		}
	}

	return "", nil, false
}

// GetAllConfigs 获取指定文件的所有配置
func (cm *ConfigManager) GetAllConfigs(filename string) (map[string]interface{}, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	configMap, exists := cm.configs[filename]
	return configMap, exists
}

// ReloadConfig 重新加载指定配置文件
func (cm *ConfigManager) ReloadConfig(filename string) error {
	return cm.LoadConfig(filename)
}

// ReloadAll 重新加载所有配置文件
func (cm *ConfigManager) ReloadAll() error {
	cm.mu.Lock()
	filenames := make([]string, 0, len(cm.configs))
	for filename := range cm.configs {
		filenames = append(filenames, filename)
	}
	cm.mu.Unlock()

	for _, filename := range filenames {
		if err := cm.ReloadConfig(filename); err != nil {
			return err
		}
	}
	return nil
}

// ListLoadedFiles 列出已加载的配置文件
func (cm *ConfigManager) ListLoadedFiles() []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	files := make([]string, 0, len(cm.configs))
	for filename := range cm.configs {
		files = append(files, filename)
	}
	return files
}

// LoadAllConfigs 加载指定目录下的所有JSON文件
func (cm *ConfigManager) LoadAllConfigs() error {
	return filepath.Walk(cm.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && filepath.Ext(path) == ".json" {
			relPath, err := filepath.Rel(cm.baseDir, path)
			if err != nil {
				return err
			}

			if err := cm.LoadConfig(relPath); err != nil {
				return fmt.Errorf("加载配置文件失败 %s: %v", relPath, err)
			}
		}

		return nil
	})
}
