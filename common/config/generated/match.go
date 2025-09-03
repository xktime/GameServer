package config

import (
	"fmt"
	"gameserver/common/config"
	"reflect"
	"sync"
)

// Match match.json配置结构体
type Match struct {
	Id float64 `json:"id"` // id
	Name string `json:"name"` // name
	Room float64 `json:"room_size"` // room_size
}

// MatchCache match.json配置缓存
type MatchCache struct {
	cache map[string]*Match
	mu    sync.RWMutex
}

var MatchCacheInstance = &MatchCache{
	cache: make(map[string]*Match),
}

// getMatchFromCache 从缓存获取配置
func (c *MatchCache) getMatchFromCache(id string) (*Match, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	if item, exists := c.cache[id]; exists {
		return item, true
	}
	return nil, false
}

// setMatchToCache 设置配置到缓存
func (c *MatchCache) setMatchToCache(id string, item *Match) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.cache[id] = item
}

// clearMatchCache 清空缓存
func (c *MatchCache) clearMatchCache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.cache = make(map[string]*Match)
}

// convertToMatch 将原始配置转换为Match结构体
func convertToMatch(config interface{}) (*Match, bool) {
	if configMap, ok := config.(map[string]interface{}); ok {
		result := &Match{}
		
		// 使用反射设置字段值
		configValue := reflect.ValueOf(result).Elem()
		configType := configValue.Type()
		
		for i := 0; i < configValue.NumField(); i++ {
			field := configValue.Field(i)
			fieldType := configType.Field(i)
			jsonTag := fieldType.Tag.Get("json")
			
			if value, exists := configMap[jsonTag]; exists {
				// 根据字段类型进行类型转换
				switch field.Kind() {
				case reflect.String:
					if str, ok := value.(string); ok {
						field.SetString(str)
					}
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					if num, ok := value.(float64); ok {
						field.SetInt(int64(num))
					}
				case reflect.Float32, reflect.Float64:
					if num, ok := value.(float64); ok {
						field.SetFloat(num)
					}
				case reflect.Bool:
					if b, ok := value.(bool); ok {
						field.SetBool(b)
					}
				case reflect.Slice:
					if slice, ok := value.([]interface{}); ok {
						// 处理字符串切片
						if field.Type().Elem().Kind() == reflect.String {
							strSlice := make([]string, len(slice))
							for j, item := range slice {
								if str, ok := item.(string); ok {
									strSlice[j] = str
								}
							}
							field.Set(reflect.ValueOf(strSlice))
						}
					}
				}
			}
		}
		
		return result, true
	}

	return nil, false
}

// GetMatchConfig 获取match.json配置（带缓存）
func GetMatchConfig(id string) (*Match, bool) {
	// 先从缓存获取
	if item, exists := MatchCacheInstance.getMatchFromCache(id); exists {
		return item, true
	}
	
	// 缓存未命中，从原始配置获取
	config, exists := config.GetConfig("match.json", id)
	if !exists {
		return nil, false
	}

	// 转换为结构体
	if item, ok := convertToMatch(config); ok {
		// 设置到缓存
		MatchCacheInstance.setMatchToCache(id, item)
		return item, true
	}

	return nil, false
}

// GetAllMatchConfigs 获取所有match.json配置（带缓存）
func GetAllMatchConfigs() (map[string]*Match, bool) {
	configs, exists := config.GetAllConfigs("match.json")
	if !exists {
		return nil, false
	}

	result := make(map[string]*Match)
	for id := range configs {
		if item, ok := GetMatchConfig(id); ok {
			result[id] = item
		}
	}

	return result, true
}

// GetMatchName 获取match.json名称
func GetMatchName(id string) (string, bool) {
	if item, exists := GetMatchConfig(id); exists {
		return item.Name, true
	}
	return "", false
}

// ReloadMatchConfig 重新加载match.json配置并清空缓存
func ReloadMatchConfig() error {
	// 清空缓存
	MatchCacheInstance.clearMatchCache()
	
	// 重新加载配置
	return config.ReloadConfig("match.json")
}

// ValidateMatchConfig 验证match.json配置
func ValidateMatchConfig(id string) error {
	if _, exists := GetMatchConfig(id); !exists {
		return fmt.Errorf("配置不存在: %s", id)
	}
	return nil
}

// ClearMatchCache 手动清空match.json配置缓存
func ClearMatchCache() {
	MatchCacheInstance.clearMatchCache()
}
