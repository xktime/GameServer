package config

import (
	"encoding/json"
	"fmt"
	"gameserver/core/log"
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
		} else {
			log.Error("配置文件 %s 缺少ID字段", filePath)
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
	// 先重新加载原始配置文件
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

	// 然后调用每个生成文件的 reload 方法
	if err := reloadAllGeneratedConfigs(); err != nil {
		return fmt.Errorf("重新加载生成配置失败: %v", err)
	}

	return nil
}

// reloadAllGeneratedConfigs 重新加载所有生成的配置文件
func reloadAllGeneratedConfigs() error {
	// 由于生成的配置文件在不同的包中，我们需要通过其他方式来调用它们的 reload 方法
	// 这里我们提供一个通用的机制，让外部代码可以注册自己的 reload 函数

	// 调用所有注册的 reload 函数
	for _, reloadFunc := range registeredReloadFuncs {
		if err := reloadFunc(); err != nil {
			return fmt.Errorf("调用注册的 reload 函数失败: %v", err)
		}
	}

	return nil
}

// ReloadFunc 重新加载配置的函数类型
type ReloadFunc func() error

// registeredReloadFuncs 存储所有注册的 reload 函数
var registeredReloadFuncs []ReloadFunc

// RegisterReloadFunc 注册一个重新加载配置的函数
func RegisterReloadFunc(reloadFunc ReloadFunc) {
	registeredReloadFuncs = append(registeredReloadFuncs, reloadFunc)
}

// UnregisterAllReloadFuncs 清空所有注册的 reload 函数
func UnregisterAllReloadFuncs() {
	registeredReloadFuncs = nil
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
