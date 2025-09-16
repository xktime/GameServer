package actor

import (
	"context"
	"testing"
	"time"
)

// 测试超时机制
func TestTaskTimeout(t *testing.T) {
	// 初始化Actor管理器，设置2秒超时
	Init(200) // 2秒超时

	// 创建一个测试用的TaskHandler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := &TaskHandler{
		taskQueue: make(chan *TaskQueue, 10),
		ctx:       ctx,
		cancel:    cancel,
		id:        "test_handler",
		actors:    make(map[string]IActor),
	}

	// 启动TaskHandler
	handler.Start()
	defer handler.Stop()

	// 测试用例1：正常任务，不会超时
	t.Run("NormalTask", func(t *testing.T) {
		start := time.Now()
		result := handler.SendTask(func() *Response {
			time.Sleep(100 * time.Millisecond) // 100ms任务
			return &Response{
				Result: []interface{}{"success"},
				Error:  nil,
			}
		})
		duration := time.Since(start)

		if result.Error != nil {
			t.Errorf("正常任务应该成功，但得到错误: %v", result.Error)
		}
		if duration > 500*time.Millisecond {
			t.Errorf("正常任务执行时间过长: %v", duration)
		}
	})

	// 测试用例2：超时任务
	t.Run("TimeoutTask", func(t *testing.T) {
		start := time.Now()
		result := handler.SendTask(func() *Response {
			time.Sleep(3 * time.Second) // 3秒任务，超过2秒超时
			return &Response{
				Result: []interface{}{"should not reach here"},
				Error:  nil,
			}
		})
		duration := time.Since(start)

		// 应该超时
		if result.Error == nil {
			t.Error("超时任务应该返回错误")
		}
		if !contains(result.Error.Error(), "timeout") {
			t.Errorf("错误信息应该包含'timeout'，但得到: %v", result.Error)
		}
		// 执行时间应该在2秒左右（允许一些误差）
		if duration < 1900*time.Millisecond || duration > 2200*time.Millisecond {
			t.Errorf("超时任务执行时间不符合预期: %v", duration)
		}
	})

	// 测试用例3：单个快速任务
	t.Run("SingleFastTask", func(t *testing.T) {
		result := handler.SendTask(func() *Response {
			time.Sleep(50 * time.Millisecond)
			return &Response{
				Result: []interface{}{"fast_task"},
				Error:  nil,
			}
		})

		// 任务应该成功
		if result.Error != nil {
			t.Errorf("快速任务应该成功，但得到错误: %v", result.Error)
		}

	})
}

// 测试超时配置
func TestTimeoutConfiguration(t *testing.T) {
	// 测试默认超时
	Init(3000) // 3秒超时
	if GetTaskTimeout() != 3000 {
		t.Errorf("期望超时时间3000ms，但得到: %d", GetTaskTimeout())
	}

	// 测试修改超时
	Init(5000) // 5秒超时
	if GetTaskTimeout() != 5000 {
		t.Errorf("期望超时时间5000ms，但得到: %d", GetTaskTimeout())
	}
}

// 辅助函数：检查字符串是否包含子字符串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			containsSubstring(s, substr))))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
