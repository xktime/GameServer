package config

import (
	"gameserver/common/config"
	"reflect"
	"strings"
	"sync"
	"time"
)

// GameConst 全局配置变量（自动生成，请勿手动修改）
var GameConst struct {
	EquipComposeNum int32 // equipComposeNum
	ShareReward int32 // shareReward
	SidebarReward int32 // sidebarReward
	SidebarRewardName string // sidebarRewardName
	GameIcon string // gameIcon
}

var gameConstOnce sync.Once
var gameConstLoaded bool

func init() {
	initGameConst()
}

// initGameConst 初始化GameConst配置
func initGameConst() {
	go gameConstOnce.Do(func() {
		// 等待配置管理器初始化
		for {
			if !config.IsInitialized() {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			configData, exists := config.GetConfig("GameConst.json", nil)
			if exists {
				// 配置管理器已初始化，继续处理
				processGameConstConfig(configData)
				break
			}
			// 如果配置管理器未初始化，等待一下
			time.Sleep(10 * time.Millisecond)
		}
	})
}

// processGameConstConfig 处理GameConst配置数据
func processGameConstConfig(configData interface{}) {
	// 使用反射设置字段值
	gameConstValue := reflect.ValueOf(&GameConst).Elem()
	gameConstType := gameConstValue.Type()
	
	for i := 0; i < gameConstValue.NumField(); i++ {
		field := gameConstValue.Field(i)
		fieldType := gameConstType.Field(i)
		fieldName := fieldType.Name
		
		// 将字段名转换为JSON字段名（首字母小写）
		jsonName := strings.ToLower(fieldName[:1]) + fieldName[1:]
		
		if value, exists := configData.(map[string]interface{})[jsonName]; exists {
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
			}
		}
	}
	
	gameConstLoaded = true
}


// ReloadGameConst 重新加载GameConst配置
func ReloadGameconstConfig() error {
	// 重新加载配置
	if err := config.ReloadConfig("GameConst.json"); err != nil {
		return err
	}
	
	// 重置once，允许重新初始化
	gameConstOnce = sync.Once{}
	gameConstLoaded = false
	
	// 重新初始化
	initGameConst()
	
	return nil
}

// IsGameConstLoaded 检查GameConst是否已加载
func IsGameConstLoaded() bool {
	if !gameConstLoaded {
		initGameConst()
	}
	return gameConstLoaded
}

