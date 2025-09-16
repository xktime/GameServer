package config

import (
	"encoding/json"
	"fmt"
	"gameserver/core/log"
	"os"
	"path/filepath"
	"sync"
)

// ConfigManager 配置管理器
type ConfigManager struct {
	configs     map[string]map[interface{}]interface{} // 文件名 -> {ID -> 配置数据}
	mu          sync.RWMutex
	baseDir     string
	initialized bool
}

// NewConfigManager 创建新的配置管理器
func NewConfigManager(baseDir string) *ConfigManager {
	return &ConfigManager{
		configs: make(map[string]map[interface{}]interface{}),
		baseDir: baseDir,
	}
}

func (cm *ConfigManager) IsInitialized() bool {
	if cm == nil {
		return false
	}
	return cm.initialized
}

func (cm *ConfigManager) SetInitialized() {
	cm.initialized = true
}

// LoadConfig 加载指定JSON配置文件
func (cm *ConfigManager) LoadConfig(filename string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	filePath := filepath.Join(cm.baseDir, filename)

	// 读取文件
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败 %s: %v", filePath, err)
	}

	// 特殊处理GameConst.json文件
	if filename == "GameConst.json" {
		return cm.loadGameConstConfig(data, filename)
	}

	// 尝试解析为新的配置格式（最外层key是配置ID）
	var configObj map[string]interface{}
	if err := json.Unmarshal(data, &configObj); err == nil {
		// 检查是否是新的配置格式：最外层key是配置ID，value是配置对象
		if cm.isNewConfigFormat(configObj) {
			return cm.loadNewFormatConfig(configObj, filename)
		}
	}

	// 尝试解析为旧的JSON数组格式
	var configArray []map[string]interface{}
	if err := json.Unmarshal(data, &configArray); err != nil {
		return fmt.Errorf("解析JSON失败 %s: %v", filePath, err)
	}

	// 创建ID到配置的映射
	configMap := make(map[interface{}]interface{})
	for _, item := range configArray {
		if id, ok := item["id"]; ok {
			// 对ID进行类型转换：整数使用int32，小数使用float64
			convertedID := convertIDType(id)
			configMap[convertedID] = item
		} else {
			log.Error("配置文件 %s 缺少ID字段", filePath)
		}
	}

	cm.configs[filename] = configMap
	return nil
}

// loadGameConstConfig 加载GameConst.json配置文件（特殊格式处理）
func (cm *ConfigManager) loadGameConstConfig(data []byte, filename string) error {
	// 解析JSON对象
	var configObj map[string]interface{}
	if err := json.Unmarshal(data, &configObj); err != nil {
		return fmt.Errorf("解析GameConst.json失败: %v", err)
	}

	// 创建配置映射，使用"default"作为默认ID
	configMap := make(map[interface{}]interface{})
	configMap["default"] = configObj

	cm.configs[filename] = configMap
	return nil
}

// isNewConfigFormat 检查是否是新的配置格式（最外层key是配置ID）
func (cm *ConfigManager) isNewConfigFormat(configObj map[string]interface{}) bool {
	// 如果map为空，不是新格式
	if len(configObj) == 0 {
		return false
	}

	// 检查第一个值是否是对象，且包含id字段
	for _, value := range configObj {
		if _, ok := value.(map[string]interface{}); ok {
			return true
		}
		break // 只检查第一个值
	}

	return false
}

// loadNewFormatConfig 加载新格式的配置文件
func (cm *ConfigManager) loadNewFormatConfig(configObj map[string]interface{}, filename string) error {
	configMap := make(map[interface{}]interface{})

	for keyStr, value := range configObj {
		if configItem, ok := value.(map[string]interface{}); ok {
			// 对key进行类型转换
			convertedKey := convertIDType(keyStr)
			configMap[convertedKey] = configItem
		}
	}

	cm.configs[filename] = configMap
	return nil
}

// GetConfig 根据文件名和ID获取配置
func (cm *ConfigManager) GetConfig(filename string, id interface{}) (interface{}, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	configMap, exists := cm.configs[filename]
	if !exists {
		return nil, false
	}

	// 特殊处理GameConst.json：如果没有指定ID或ID为nil，返回默认配置
	if filename == "GameConst.json" {
		if id == nil {
			config, exists := configMap["default"]
			return config, exists
		}
	}

	config, exists := configMap[id]
	return config, exists
}

// GetGameConstConfig 获取GameConst配置（便捷方法）
func (cm *ConfigManager) GetGameConstConfig() (map[string]interface{}, bool) {
	config, exists := cm.GetConfig("GameConst.json", nil)
	if !exists {
		return nil, false
	}

	if configMap, ok := config.(map[string]interface{}); ok {
		return configMap, true
	}

	return nil, false
}

// GetConfigByID 根据ID获取配置（自动查找所有已加载的文件）
func (cm *ConfigManager) GetConfigByID(id interface{}) (string, interface{}, bool) {
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
func (cm *ConfigManager) GetAllConfigs(filename string) (map[interface{}]interface{}, bool) {
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
				log.Error("加载配置文件失败 %s: %v", relPath, err)
			} else if err := cm.LoadConfig(relPath); err != nil {
				log.Error("加载配置文件失败 %s: %v", relPath, err)
			}
		}

		return nil
	})
}

// convertIDType 转换ID类型：整数使用int32，小数使用float64
func convertIDType(id interface{}) interface{} {
	switch v := id.(type) {
	case float64:
		// 如果值是整数（没有小数点），使用int32
		if v == float64(int64(v)) {
			return int32(v)
		}
		// 只有真正有小数点的数值才使用float64
		return v
	case int:
		return int32(v)
	case int32:
		return v
	case int64:
		return int32(v)
	case string:
		return v
	default:
		return v
	}
}
