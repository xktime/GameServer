package config

import (
	"fmt"
	"gameserver/common/config"
	"reflect"
	"sync"
)

// RechargeData recharge.json配置数据结构体（自动生成，请勿手动修改）
type RechargeData struct {
	Id string `json:"id"` // id
	Name string `json:"name"` // name
	Description string `json:"description"` // description
	Amount int32 `json:"amount"` // amount
	Bonus int32 `json:"bonus"` // bonus
	Currency string `json:"currency"` // currency
	Is bool `json:"is_active"` // is_active
	Sort int32 `json:"sort_order"` // sort_order
}

// Recharge recharge.json配置结构体（可添加自定义方法）
type Recharge struct {
	*RechargeData
}

// NewRecharge 创建新的Recharge实例
func NewRecharge(data *RechargeData) *Recharge {
	return &Recharge{
		RechargeData: data,
	}
}

// RechargeCache recharge.json配置缓存
type RechargeCache struct {
	cache map[string]*RechargeData
	mu    sync.RWMutex
}

var RechargeCacheInstance = &RechargeCache{
	cache: make(map[string]*RechargeData),
}

// getRechargeFromCache 从缓存获取配置
func (c *RechargeCache) getRechargeFromCache(id string) (*RechargeData, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	if item, exists := c.cache[id]; exists {
		return item, true
	}
	return nil, false
}

// setRechargeToCache 设置配置到缓存
func (c *RechargeCache) setRechargeToCache(id string, item *RechargeData) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.cache[id] = item
}

// clearRechargeCache 清空缓存
func (c *RechargeCache) clearRechargeCache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.cache = make(map[string]*RechargeData)
}

// convertToRecharge 将原始配置转换为RechargeData结构体
func convertToRecharge(config interface{}) (*RechargeData, bool) {
	if configMap, ok := config.(map[string]interface{}); ok {
		result := &RechargeData{}
		
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
						// 处理不同类型的切片
						elemType := field.Type().Elem()
						switch elemType.Kind() {
						case reflect.String:
							strSlice := make([]string, len(slice))
							for j, item := range slice {
								if str, ok := item.(string); ok {
									strSlice[j] = str
								}
							}
							field.Set(reflect.ValueOf(strSlice))
						case reflect.Int32:
							intSlice := make([]int32, len(slice))
							for j, item := range slice {
								if num, ok := item.(float64); ok {
									intSlice[j] = int32(num)
								}
							}
							field.Set(reflect.ValueOf(intSlice))
						case reflect.Float64:
							floatSlice := make([]float64, len(slice))
							for j, item := range slice {
								if num, ok := item.(float64); ok {
									floatSlice[j] = num
								}
							}
							field.Set(reflect.ValueOf(floatSlice))
						case reflect.Bool:
							boolSlice := make([]bool, len(slice))
							for j, item := range slice {
								if b, ok := item.(bool); ok {
									boolSlice[j] = b
								}
							}
							field.Set(reflect.ValueOf(boolSlice))
						case reflect.Slice:
							// 处理二维数组
							if elemType.Elem().Kind() == reflect.String {
								// [][]string
								str2DSlice := make([][]string, len(slice))
								for i, outerItem := range slice {
									if outerSlice, ok := outerItem.([]interface{}); ok {
										strSlice := make([]string, len(outerSlice))
										for j, innerItem := range outerSlice {
											if str, ok := innerItem.(string); ok {
												strSlice[j] = str
											}
										}
										str2DSlice[i] = strSlice
									}
								}
								field.Set(reflect.ValueOf(str2DSlice))
							} else if elemType.Elem().Kind() == reflect.Int32 {
								// [][]int32
								int2DSlice := make([][]int32, len(slice))
								for i, outerItem := range slice {
									if outerSlice, ok := outerItem.([]interface{}); ok {
										intSlice := make([]int32, len(outerSlice))
										for j, innerItem := range outerSlice {
											if num, ok := innerItem.(float64); ok {
												intSlice[j] = int32(num)
											}
										}
										int2DSlice[i] = intSlice
									}
								}
								field.Set(reflect.ValueOf(int2DSlice))
							} else if elemType.Elem().Kind() == reflect.Float64 {
								// [][]float64
								float2DSlice := make([][]float64, len(slice))
								for i, outerItem := range slice {
									if outerSlice, ok := outerItem.([]interface{}); ok {
										floatSlice := make([]float64, len(outerSlice))
										for j, innerItem := range outerSlice {
											if num, ok := innerItem.(float64); ok {
												floatSlice[j] = num
											}
										}
										float2DSlice[i] = floatSlice
									}
								}
								field.Set(reflect.ValueOf(float2DSlice))
							} else {
								// 其他二维数组类型，使用interface{}
								field.Set(reflect.ValueOf(slice))
							}
						default:
							// 对于复杂类型，直接设置为 interface{}
							field.Set(reflect.ValueOf(slice))
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
	if itemData, exists := RechargeCacheInstance.getRechargeFromCache(id); exists {
		return NewRecharge(itemData), true
	}
	
	// 缓存未命中，从原始配置获取
	config, exists := config.GetConfig("recharge.json", id)
	if !exists {
		return nil, false
	}

	// 转换为结构体
	if itemData, ok := convertToRecharge(config); ok {
		// 设置到缓存
		RechargeCacheInstance.setRechargeToCache(id, itemData)
		return NewRecharge(itemData), true
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
	for idInterface := range configs {
		// 类型转换 - 现在ID已经是正确的类型了
		var id string
		var ok bool
		
		id, ok = idInterface.(string)
		
		
		if !ok {
			continue // 类型转换失败，跳过
		}
		
		if item, ok := GetRechargeConfig(id); ok {
			result[id] = item
		}
	}

	return result, true
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
		return fmt.Errorf("配置不存在: %v", id)
	}
	return nil
}

// ClearRechargeCache 手动清空recharge.json配置缓存
func ClearRechargeCache() {
	RechargeCacheInstance.clearRechargeCache()
}
