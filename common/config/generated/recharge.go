package config

import (
	"fmt"
	"gameserver/common/config"
	"reflect"
	"sync"
)

// Recharge recharge.json配置结构体
type Recharge struct {
	Id string `json:"id"` // id
	Name string `json:"name"` // name
	Description string `json:"description"` // description
	Amount int64 `json:"amount"` // amount
	Bonus int64 `json:"bonus"` // bonus
	Currency string `json:"currency"` // currency
	Is bool `json:"is_active"` // is_active
	Sort int64 `json:"sort_order"` // sort_order
}

// RechargeCache recharge.json配置缓存
type RechargeCache struct {
	cache map[string]*Recharge
	mu    sync.RWMutex
}

var RechargeCacheInstance = &RechargeCache{
	cache: make(map[string]*Recharge),
}

// getRechargeFromCache 从缓存获取配置
func (c *RechargeCache) getRechargeFromCache(id string) (*Recharge, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	if item, exists := c.cache[id]; exists {
		return item, true
	}
	return nil, false
}

// setRechargeToCache 设置配置到缓存
func (c *RechargeCache) setRechargeToCache(id string, item *Recharge) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.cache[id] = item
}

// clearRechargeCache 清空缓存
func (c *RechargeCache) clearRechargeCache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.cache = make(map[string]*Recharge)
}

// convertToRecharge 将原始配置转换为Recharge结构体
func convertToRecharge(config interface{}) (*Recharge, bool) {
	if configMap, ok := config.(map[string]interface{}); ok {
		result := &Recharge{}
		
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

// GetRechargeConfig 获取recharge.json配置（带缓存）
func GetRechargeConfig(id string) (*Recharge, bool) {
	// 先从缓存获取
	if item, exists := RechargeCacheInstance.getRechargeFromCache(id); exists {
		return item, true
	}
	
	// 缓存未命中，从原始配置获取
	config, exists := config.GetConfig("recharge.json", id)
	if !exists {
		return nil, false
	}

	// 转换为结构体
	if item, ok := convertToRecharge(config); ok {
		// 设置到缓存
		RechargeCacheInstance.setRechargeToCache(id, item)
		return item, true
	}

	return nil, false
}

// GetAllRechargeConfigs 获取所有recharge.json配置（带缓存）
func GetAllRechargeConfigs() (map[string]*Recharge, bool) {
	configs, exists := config.GetAllConfigs("recharge.json")
	if !exists {
		return nil, false
	}

	result := make(map[string]*Recharge)
	for id := range configs {
		if item, ok := GetRechargeConfig(id); ok {
			result[id] = item
		}
	}

	return result, true
}

// GetRechargeName 获取recharge.json名称
func GetRechargeName(id string) (string, bool) {
	if item, exists := GetRechargeConfig(id); exists {
		return item.Name, true
	}
	return "", false
}

// ReloadRechargeConfig 重新加载recharge.json配置并清空缓存
func ReloadRechargeConfig() error {
	// 清空缓存
	RechargeCacheInstance.clearRechargeCache()
	
	// 重新加载配置
	return config.ReloadConfig("recharge.json")
}

// ValidateRechargeConfig 验证recharge.json配置
func ValidateRechargeConfig(id string) error {
	if _, exists := GetRechargeConfig(id); !exists {
		return fmt.Errorf("配置不存在: %s", id)
	}
	return nil
}

// ClearRechargeCache 手动清空recharge.json配置缓存
func ClearRechargeCache() {
	RechargeCacheInstance.clearRechargeCache()
}
