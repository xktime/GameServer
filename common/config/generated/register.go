package config

import (
	"gameserver/common/config"
)

// init 函数在包初始化时自动执行
func init() {
	// 注册所有生成的配置 reload 函数
	// 这样当调用 config.ReloadAll() 时，会自动调用这些函数
	
	// 注册Item配置重载函数
	config.RegisterReloadFunc(func() error {
		return ReloadItemConfig()
	})
	// 注册Match配置重载函数
	config.RegisterReloadFunc(func() error {
		return ReloadMatchConfig()
	})
	// 注册Monster配置重载函数
	config.RegisterReloadFunc(func() error {
		return ReloadMonsterConfig()
	})
	// 注册Recharge配置重载函数
	config.RegisterReloadFunc(func() error {
		return ReloadRechargeConfig()
	})
	// 注册Skill配置重载函数
	config.RegisterReloadFunc(func() error {
		return ReloadSkillConfig()
	})

}
