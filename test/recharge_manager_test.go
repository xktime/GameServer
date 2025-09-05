package test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"gameserver/common/base/actor"
	"gameserver/common/msg/message"

	"github.com/stretchr/testify/assert"
)

// MockAgent 模拟 Agent 接口
type MockAgent struct {
	userData    interface{}
	writtenMsgs []interface{}
}

func (m *MockAgent) WriteMsg(msg interface{}) {
	m.writtenMsgs = append(m.writtenMsgs, msg)
}

func (m *MockAgent) UserData() interface{} {
	return m.userData
}

func (m *MockAgent) SetUserData(data interface{}) {
	m.userData = data
}

func (m *MockAgent) Close() {
	// 模拟关闭
}

func (m *MockAgent) GetWrittenMsgs() []interface{} {
	return m.writtenMsgs
}

// TestManager 用于测试的 Manager 结构
type TestManager struct {
	*actor.TaskHandler
}

// Init 实现 IActor 接口
func (m *TestManager) Init() {
	// 测试用的初始化方法
}

// Stop 实现 IActor 接口
func (m *TestManager) Stop() {
	// 测试用的停止方法
}

// setupTest 设置测试环境
func setupTest() {
	actor.Init(1000)
}

// TestTaskHandlerBasic 测试 TaskHandler 基本功能
func TestTaskHandlerBasic(t *testing.T) {
	setupTest()

	manager := &TestManager{}
	manager.TaskHandler = actor.InitTaskHandler(actor.Recharge, "test", manager)
	manager.TaskHandler.Start()
	defer manager.TaskHandler.Stop()

	// 测试 SendTask 基本功能
	response := manager.SendTask(func() *actor.Response {
		return &actor.Response{
			Result: []interface{}{"test_result"},
		}
	})

	assert.NotNil(t, response, "Response 不应为 nil")
	assert.Len(t, response.Result, 1, "应该有一个结果")
	assert.Equal(t, "test_result", response.Result[0], "结果应该匹配")
}

// TestTaskHandlerConcurrency 测试 TaskHandler 并发安全性
func TestTaskHandlerConcurrency(t *testing.T) {
	setupTest()

	manager := &TestManager{}
	manager.TaskHandler = actor.InitTaskHandler(actor.Recharge, "test_concurrent", manager)
	manager.TaskHandler.Start()
	defer manager.TaskHandler.Stop()

	// 并发测试
	const numGoroutines = 10
	const numRequests = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < numRequests; j++ {
				// 测试 SendTask 的并发安全性
				response := manager.SendTask(func() *actor.Response {
					return &actor.Response{
						Result: []interface{}{goroutineID, j},
					}
				})

				assert.NotNil(t, response, "Response 不应为 nil")
				assert.Len(t, response.Result, 2, "应该有两个结果")
			}
		}(i)
	}

	wg.Wait()
}

// TestTaskHandlerInitAndStop 测试初始化和停止
func TestTaskHandlerInitAndStop(t *testing.T) {
	setupTest()

	manager := &TestManager{}

	// 测试初始化
	manager.TaskHandler = actor.InitTaskHandler(actor.Recharge, "test_lifecycle", manager)
	manager.TaskHandler.Start()
	assert.NotNil(t, manager.TaskHandler, "TaskHandler 应该被初始化")

	// 测试在初始化后可以正常使用
	response := manager.SendTask(func() *actor.Response {
		return &actor.Response{
			Result: []interface{}{"lifecycle_test"},
		}
	})
	assert.NotNil(t, response, "初始化后应该可以正常使用")

	// 测试停止
	manager.TaskHandler.Stop()

	// 注意：停止后再次调用 SendTask 可能会失败，这是预期的行为
	// 这里我们主要测试停止方法不会 panic
	assert.NotPanics(t, func() {
		manager.TaskHandler.Stop() // 重复停止不应该 panic
	}, "重复停止不应该 panic")
}

// TestTaskHandlerErrorHandling 测试错误处理
func TestTaskHandlerErrorHandling(t *testing.T) {
	setupTest()

	manager := &TestManager{}
	manager.TaskHandler = actor.InitTaskHandler(actor.Recharge, "test_error", manager)
	manager.TaskHandler.Start()
	defer manager.TaskHandler.Stop()

	// 测试返回错误的 Response
	response := manager.SendTask(func() *actor.Response {
		return &actor.Response{
			Result: []interface{}{},
			Error:  assert.AnError,
		}
	})

	assert.NotNil(t, response, "Response 不应为 nil")
	assert.Error(t, response.Error, "应该包含错误")
}

// TestTaskHandlerMultipleTasks 测试多个异步任务
func TestTaskHandlerMultipleTasks(t *testing.T) {
	setupTest()

	manager := &TestManager{}
	manager.TaskHandler = actor.InitTaskHandler(actor.Recharge, "test_multiple", manager)
	manager.TaskHandler.Start()
	defer manager.TaskHandler.Stop()

	// 测试多个异步任务
	responses := make([]*actor.Response, 5)

	for i := 0; i < 5; i++ {
		finalI := i // 捕获循环变量
		response := manager.SendTask(func() *actor.Response {
			time.Sleep(10 * time.Millisecond) // 模拟一些工作
			return &actor.Response{
				Result: []interface{}{finalI},
			}
		})
		responses[finalI] = response
	}

	// 验证所有响应
	for i, response := range responses {
		assert.NotNil(t, response, "Response %d 不应为 nil", i)
		assert.Len(t, response.Result, 1, "Response %d 应该有一个结果", i)
		assert.Equal(t, i, response.Result[0], "Response %d 的结果应该匹配", i)
	}
}

// TestTaskHandlerWithComplexData 测试复杂数据结构
func TestTaskHandlerWithComplexData(t *testing.T) {
	setupTest()

	manager := &TestManager{}
	manager.TaskHandler = actor.InitTaskHandler(actor.Recharge, "test_complex", manager)
	manager.TaskHandler.Start()
	defer manager.TaskHandler.Stop()

	// 测试复杂数据结构
	type TestData struct {
		ID       int64
		Name     string
		Amount   int64
		Platform message.PaymentPlatform
	}

	testData := TestData{
		ID:       12345,
		Name:     "test_user",
		Amount:   1000,
		Platform: message.PaymentPlatform_Platform_WeChat,
	}

	response := manager.SendTask(func() *actor.Response {
		return &actor.Response{
			Result: []interface{}{testData},
		}
	})

	assert.NotNil(t, response, "Response 不应为 nil")
	assert.Len(t, response.Result, 1, "应该有一个结果")

	resultData, ok := response.Result[0].(TestData)
	assert.True(t, ok, "结果应该是 TestData 类型")
	assert.Equal(t, testData.ID, resultData.ID, "ID 应该匹配")
	assert.Equal(t, testData.Name, resultData.Name, "Name 应该匹配")
	assert.Equal(t, testData.Amount, resultData.Amount, "Amount 应该匹配")
	assert.Equal(t, testData.Platform, resultData.Platform, "Platform 应该匹配")
}

// TestTaskHandlerPerformance 性能测试
func TestTaskHandlerPerformance(t *testing.T) {
	setupTest()

	manager := &TestManager{}
	manager.TaskHandler = actor.InitTaskHandler(actor.Recharge, "test_performance", manager)
	manager.TaskHandler.Start()
	defer manager.TaskHandler.Stop()

	// 性能测试
	const numTasks = 1000
	start := time.Now()

	for i := 0; i < numTasks; i++ {
		response := manager.SendTask(func() *actor.Response {
			return &actor.Response{
				Result: []interface{}{"performance_test"},
			}
		})
		assert.NotNil(t, response, "Response 不应为 nil")
	}

	duration := time.Since(start)
	t.Logf("处理 %d 个任务耗时: %v", numTasks, duration)
	t.Logf("平均每个任务耗时: %v", duration/numTasks)

	// 验证性能在合理范围内（每个任务不超过 1ms）
	assert.Less(t, duration/numTasks, time.Millisecond, "每个任务的处理时间应该小于 1ms")
}

// BenchmarkTaskHandlerSendTask 性能基准测试
func BenchmarkTaskHandlerSendTask(b *testing.B) {
	setupTest()

	manager := &TestManager{}
	manager.TaskHandler = actor.InitTaskHandler(actor.Recharge, "benchmark", manager)
	manager.TaskHandler.Start()
	defer manager.TaskHandler.Stop()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			manager.SendTask(func() *actor.Response {
				return &actor.Response{
					Result: []interface{}{"benchmark_result"},
				}
			})
		}
	})
}

// TestTaskHandlerLifecycle 测试生命周期管理
func TestTaskHandlerLifecycle(t *testing.T) {
	setupTest()

	// 测试创建多个 TaskHandler 实例
	managers := make([]*TestManager, 3)

	for i := 0; i < 3; i++ {
		manager := &TestManager{}
		manager.TaskHandler = actor.InitTaskHandler(actor.Recharge, fmt.Sprintf("test_lifecycle_%d", i), manager)
		manager.TaskHandler.Start()
		managers[i] = manager
	}

	// 测试所有实例都能正常工作
	for i, manager := range managers {
		response := manager.SendTask(func() *actor.Response {
			return &actor.Response{
				Result: []interface{}{i},
			}
		})
		assert.NotNil(t, response, "Manager %d 应该正常工作", i)
	}

	// 测试停止所有实例
	for i, manager := range managers {
		assert.NotPanics(t, func() {
			manager.TaskHandler.Stop()
		}, "停止 Manager %d 不应该 panic", i)
	}
}
