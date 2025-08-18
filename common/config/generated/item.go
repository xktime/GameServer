package config

import (
	"fmt"
	"gameserver/common/config"
	"reflect"
	"sync"
)

// Item items.json配置结构体
type Item struct {
	Id string `json:"id"` // id
	Name string `json:"name"` // name
	Type string `json:"type"` // type
	Attack float64 `json:"attack"` // attack
	Price float64 `json:"price"` // price
	Durability float64 `json:"durability"` // durability
	Description string `json:"description"` // description
}

// ItemCache items.json配置缓存
type ItemCache struct {
	cache map[string]*Item
	mu    sync.RWMutex
}

var ItemCacheInstance = &ItemCache{
	cache: make(map[string]*Item),
}

// getItemFromCache 从缓存获取配置
func (c *ItemCache) getItemFromCache(id string) (*Item, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	if item, exists := c.cache[id]; exists {
		return item, true
	}
	return nil, false
}

// setItemToCache 设置配置到缓存
func (c *ItemCache) setItemToCache(id string, item *Item) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.cache[id] = item
}

// clearItemCache 清空缓存
func (c *ItemCache) clearItemCache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.cache = make(map[string]*Item)
}

// convertToItem 将原始配置转换为Item结构体
func convertToItem(config interface{}) (*Item, bool) {
	if configMap, ok := config.(map[string]interface{}); ok {
		result := &Item{}
		
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

// GetItemConfig 获取items.json配置（带缓存）
func GetItemConfig(id string) (*Item, bool) {
	// 先从缓存获取
	if item, exists := ItemCacheInstance.getItemFromCache(id); exists {
		return item, true
	}
	
	// 缓存未命中，从原始配置获取
	config, exists := config.GetConfig("items.json", id)
	if !exists {
		return nil, false
	}

	// 转换为结构体
	if item, ok := convertToItem(config); ok {
		// 设置到缓存
		ItemCacheInstance.setItemToCache(id, item)
		return item, true
	}

	return nil, false
}

// GetAllItemConfigs 获取所有items.json配置（带缓存）
func GetAllItemConfigs() (map[string]*Item, bool) {
	configs, exists := config.GetAllConfigs("items.json")
	if !exists {
		return nil, false
	}

	result := make(map[string]*Item)
	for id := range configs {
		if item, ok := GetItemConfig(id); ok {
			result[id] = item
		}
	}

	return result, true
}

// GetItemName 获取items.json名称
func GetItemName(id string) (string, bool) {
	if item, exists := GetItemConfig(id); exists {
		return item.Name, true
	}
	return "", false
}

// ReloadItemConfig 重新加载items.json配置并清空缓存
func ReloadItemConfig() error {
	// 清空缓存
	ItemCacheInstance.clearItemCache()
	
	// 重新加载配置
	return config.ReloadConfig("items.json")
}

// ValidateItemConfig 验证items.json配置
func ValidateItemConfig(id string) error {
	if _, exists := GetItemConfig(id); !exists {
		return fmt.Errorf("配置不存在: %s", id)
	}
	return nil
}

// ClearItemCache 手动清空items.json配置缓存
func ClearItemCache() {
	ItemCacheInstance.clearItemCache()
}
