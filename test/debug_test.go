package test

import (
	"gameserver/conf"
	"testing"
)

func TestDebugConfig(t *testing.T) {
	// 测试启用debug的配置
	t.Run("DebugEnabled", func(t *testing.T) {
		conf.Instance().Init("../conf")

		if !conf.Server.Debug.Enabled {
			t.Error("Debug配置未启用")
		}

		if conf.Server.Debug.Port != 6060 {
			t.Errorf("Debug端口配置错误，期望: 6060, 实际: %d", conf.Server.Debug.Port)
		}

		if conf.Server.LogLevel != "debug" {
			t.Errorf("日志级别配置错误，期望: debug, 实际: %s", conf.Server.LogLevel)
		}

		if conf.Server.MachineID != 1 {
			t.Errorf("机器ID配置错误，期望: 1, 实际: %d", conf.Server.MachineID)
		}

		if conf.Server.Actor.TimeoutMillisecond != 2000 {
			t.Errorf("Actor超时配置错误，期望: 2000, 实际: %d", conf.Server.Actor.TimeoutMillisecond)
		}
	})

	t.Log("Debug配置测试完成")
}
