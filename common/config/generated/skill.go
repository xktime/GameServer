package config

import (
	"fmt"
	"gameserver/common/config"
	"reflect"
	"sync"
)

// Skill skills.json配置结构体
type Skill struct {
	Id string `json:"id"` // id
	Name string `json:"name"` // name
	Type string `json:"type"` // type
	Level float64 `json:"level"` // level
	Description string `json:"description"` // description
	Cooldown float64 `json:"cooldown"` // cooldown
	Range float64 `json:"range"` // range
	Unlock float64 `json:"unlock_level"` // unlock_level
	Damage float64 `json:"damage"` // damage
	Mana float64 `json:"mana_cost"` // mana_cost
	Effects []string `json:"effects"` // effects
}

// SkillCache skills.json配置缓存
type SkillCache struct {
	cache map[string]*Skill
	mu    sync.RWMutex
}

var SkillCacheInstance = &SkillCache{
	cache: make(map[string]*Skill),
}

// getSkillFromCache 从缓存获取配置
func (c *SkillCache) getSkillFromCache(id string) (*Skill, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	if item, exists := c.cache[id]; exists {
		return item, true
	}
	return nil, false
}

// setSkillToCache 设置配置到缓存
func (c *SkillCache) setSkillToCache(id string, item *Skill) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.cache[id] = item
}

// clearSkillCache 清空缓存
func (c *SkillCache) clearSkillCache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.cache = make(map[string]*Skill)
}

// convertToSkill 将原始配置转换为Skill结构体
func convertToSkill(config interface{}) (*Skill, bool) {
	if configMap, ok := config.(map[string]interface{}); ok {
		result := &Skill{}
		
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

// GetSkillConfig 获取skills.json配置（带缓存）
func GetSkillConfig(id string) (*Skill, bool) {
	// 先从缓存获取
	if item, exists := SkillCacheInstance.getSkillFromCache(id); exists {
		return item, true
	}
	
	// 缓存未命中，从原始配置获取
	config, exists := config.GetConfig("skills.json", id)
	if !exists {
		return nil, false
	}

	// 转换为结构体
	if item, ok := convertToSkill(config); ok {
		// 设置到缓存
		SkillCacheInstance.setSkillToCache(id, item)
		return item, true
	}

	return nil, false
}

// GetAllSkillConfigs 获取所有skills.json配置（带缓存）
func GetAllSkillConfigs() (map[string]*Skill, bool) {
	configs, exists := config.GetAllConfigs("skills.json")
	if !exists {
		return nil, false
	}

	result := make(map[string]*Skill)
	for id := range configs {
		if item, ok := GetSkillConfig(id); ok {
			result[id] = item
		}
	}

	return result, true
}

// GetSkillName 获取skills.json名称
func GetSkillName(id string) (string, bool) {
	if item, exists := GetSkillConfig(id); exists {
		return item.Name, true
	}
	return "", false
}

// ReloadSkillConfig 重新加载skills.json配置并清空缓存
func ReloadSkillConfig() error {
	// 清空缓存
	SkillCacheInstance.clearSkillCache()
	
	// 重新加载配置
	return config.ReloadConfig("skills.json")
}

// ValidateSkillConfig 验证skills.json配置
func ValidateSkillConfig(id string) error {
	if _, exists := GetSkillConfig(id); !exists {
		return fmt.Errorf("配置不存在: %s", id)
	}
	return nil
}

// ClearSkillCache 手动清空skills.json配置缓存
func ClearSkillCache() {
	SkillCacheInstance.clearSkillCache()
}
