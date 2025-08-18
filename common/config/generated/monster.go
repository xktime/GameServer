package config

import (
	"fmt"
	"gameserver/common/config"
	"reflect"
	"sync"
)

// Monster monsters.json配置结构体
type Monster struct {
	Id string `json:"id"` // id
	Name string `json:"name"` // name
	Type string `json:"type"` // type
	Level float64 `json:"level"` // level
	Hp float64 `json:"hp"` // hp
	Attack float64 `json:"attack"` // attack
	Defense float64 `json:"defense"` // defense
	Exp float64 `json:"exp"` // exp
	Drops []string `json:"drops"` // drops
}

// MonsterCache monsters.json配置缓存
type MonsterCache struct {
	cache map[string]*Monster
	mu    sync.RWMutex
}

var MonsterCacheInstance = &MonsterCache{
	cache: make(map[string]*Monster),
}

// getMonsterFromCache 从缓存获取配置
func (c *MonsterCache) getMonsterFromCache(id string) (*Monster, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	if item, exists := c.cache[id]; exists {
		return item, true
	}
	return nil, false
}

// setMonsterToCache 设置配置到缓存
func (c *MonsterCache) setMonsterToCache(id string, item *Monster) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.cache[id] = item
}

// clearMonsterCache 清空缓存
func (c *MonsterCache) clearMonsterCache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.cache = make(map[string]*Monster)
}

// convertToMonster 将原始配置转换为Monster结构体
func convertToMonster(config interface{}) (*Monster, bool) {
	if configMap, ok := config.(map[string]interface{}); ok {
		result := &Monster{}
		
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

// GetMonsterConfig 获取monsters.json配置（带缓存）
func GetMonsterConfig(id string) (*Monster, bool) {
	// 先从缓存获取
	if item, exists := MonsterCacheInstance.getMonsterFromCache(id); exists {
		return item, true
	}
	
	// 缓存未命中，从原始配置获取
	config, exists := config.GetConfig("monsters.json", id)
	if !exists {
		return nil, false
	}

	// 转换为结构体
	if item, ok := convertToMonster(config); ok {
		// 设置到缓存
		MonsterCacheInstance.setMonsterToCache(id, item)
		return item, true
	}

	return nil, false
}

// GetAllMonsterConfigs 获取所有monsters.json配置（带缓存）
func GetAllMonsterConfigs() (map[string]*Monster, bool) {
	configs, exists := config.GetAllConfigs("monsters.json")
	if !exists {
		return nil, false
	}

	result := make(map[string]*Monster)
	for id := range configs {
		if item, ok := GetMonsterConfig(id); ok {
			result[id] = item
		}
	}

	return result, true
}

// GetMonsterName 获取monsters.json名称
func GetMonsterName(id string) (string, bool) {
	if item, exists := GetMonsterConfig(id); exists {
		return item.Name, true
	}
	return "", false
}

// ReloadMonsterConfig 重新加载monsters.json配置并清空缓存
func ReloadMonsterConfig() error {
	// 清空缓存
	MonsterCacheInstance.clearMonsterCache()
	
	// 重新加载配置
	return config.ReloadConfig("monsters.json")
}

// ValidateMonsterConfig 验证monsters.json配置
func ValidateMonsterConfig(id string) error {
	if _, exists := GetMonsterConfig(id); !exists {
		return fmt.Errorf("配置不存在: %s", id)
	}
	return nil
}

// ClearMonsterCache 手动清空monsters.json配置缓存
func ClearMonsterCache() {
	MonsterCacheInstance.clearMonsterCache()
}
