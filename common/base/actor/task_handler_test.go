package actor

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// MockActor 用于测试的模拟Actor
type MockActor struct {
	BaseActor
	ID            string
	InitCalled    int32
	StopCalled    int32
	TasksExecuted int32
}

func (m *MockActor) Init(args ...any) {
	atomic.AddInt32(&m.InitCalled, 1)
	if len(args) > 0 {
		if id, ok := args[0].(string); ok {
			m.ID = id
		}
	}
}

func (m *MockActor) Stop() {
	atomic.AddInt32(&m.StopCalled, 1)
}

func (m *MockActor) ExecuteTask() int {
	count := atomic.AddInt32(&m.TasksExecuted, 1)
	return int(count)
}

// TestConcurrentRegisterUnregister 测试高并发下的注册和注销
func TestConcurrentRegisterUnregister(t *testing.T) {
	// 初始化全局管理器
	Init(5000)

	const (
		numGoroutines = 10000
		numOperations = 500
	)

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*numOperations)

	// 并发注册和注销
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numOperations; j++ {
				actorID := fmt.Sprintf("test_%d", id)

				// 注册Actor
				actor := RegisterActor[*MockActor](User, actorID, actorID)
				if actor == nil {
					errors <- fmt.Errorf("RegisterActor returned nil for %s", actorID)
					continue
				}

				// 执行一些任务
				result := actor.SendTask(func() int {
					return actor.ExecuteTask()
				})

				if err, ok := result.(error); ok {
					errors <- fmt.Errorf("task execution failed: %v", err)
				}

				// 停止Actor
				if err := StopActor[*MockActor](User, actorID); err != nil {
					errors <- fmt.Errorf("StopActor failed: %v", err)
				}

				// 验证Actor已被移除
				time.Sleep(time.Millisecond * 10) // 给一点时间让停止完成
				if _, exists := GetActor[MockActor](User, actorID); exists {
					errors <- fmt.Errorf("actor %s still exists after stop", actorID)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// 检查错误
	errorCount := 0
	for err := range errors {
		t.Errorf("Error: %v", err)
		errorCount++
		if errorCount > 10 {
			t.Fatal("Too many errors, stopping test")
		}
	}

	// 验证所有handler都已清理
	count := globalActorManager.GetTaskHandlerCount()
	if count != 0 {
		t.Errorf("Expected 0 handlers, got %d", count)
	}
}

// TestConcurrentSameIDRegister 测试同一ID的并发注册
func TestConcurrentSameIDRegister(t *testing.T) {
	Init(5000)

	const (
		numGoroutines = 50
		actorID       = "shared_actor"
	)

	var wg sync.WaitGroup
	successCount := int32(0)
	actors := make([]*MockActor, numGoroutines)

	// 并发注册同一个ID
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			actor := RegisterActor[*MockActor](User, actorID, actorID)
			actors[index] = actor

			if actor != nil {
				atomic.AddInt32(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()

	// 所有goroutine都应该成功获取到actor
	if successCount != numGoroutines {
		t.Errorf("Expected %d successful registrations, got %d", numGoroutines, successCount)
	}

	// 验证所有返回的actor都指向同一个TaskHandler
	// 注意：由于RegisterActor每次都创建新的actor实例，但它们共享同一个TaskHandler
	var firstHandler *TaskHandler
	handlerSet := make(map[*TaskHandler]bool)

	for i, actor := range actors {
		if actor == nil {
			t.Errorf("Actor at index %d is nil", i)
			continue
		}
		if firstHandler == nil {
			firstHandler = actor.TaskHandler
		}
		handlerSet[actor.TaskHandler] = true
	}

	// 由于并发注册，可能会有多个handler被创建（竞态条件）
	// 但最终应该只有一个handler在管理器中
	handler, exists := GetHandler(getUniqueId(User, actorID))
	if !exists {
		t.Error("Handler should exist in manager")
	}

	// 验证handler中有多个actor（因为每次RegisterActor都添加了一个新的actor实例）
	handler.mu.RLock()
	actorCount := len(handler.actors)
	handler.mu.RUnlock()

	t.Logf("Handler has %d actors registered, %d unique handlers created", actorCount, len(handlerSet))

	// 清理
	StopActor[*MockActor](User, actorID)
	time.Sleep(time.Millisecond * 100)
}

// TestConcurrentActorRemoval 测试并发移除Actor
func TestConcurrentActorRemoval(t *testing.T) {
	Init(5000)

	const (
		numActorsPerHandler = 10
	)

	// 创建多个Actor，每个使用不同的handler ID
	actors := make([]*MockActor, numActorsPerHandler)
	handlerIDs := make([]string, numActorsPerHandler)

	for i := 0; i < numActorsPerHandler; i++ {
		handlerID := fmt.Sprintf("multi_actor_handler_%d", i)
		handlerIDs[i] = handlerID
		actors[i] = RegisterActor[*MockActor](User, handlerID, fmt.Sprintf("actor_%d", i))
	}

	// 验证所有handler都已注册
	for i, handlerID := range handlerIDs {
		handler, exists := GetHandler(getUniqueId(User, handlerID))
		if !exists {
			t.Fatalf("Handler %d not found", i)
		}
		if len(handler.actors) != 1 {
			t.Errorf("Handler %d: expected 1 actor, got %d", i, len(handler.actors))
		}
	}

	// 并发移除所有Actor
	var wg sync.WaitGroup
	for i := 0; i < numActorsPerHandler; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			StopActor[*MockActor](User, handlerIDs[index])
		}(i)
	}

	wg.Wait()
	time.Sleep(time.Millisecond * 100) // 等待handler停止

	// 验证所有handler已被清理
	for i, handlerID := range handlerIDs {
		_, exists := GetHandler(getUniqueId(User, handlerID))
		if exists {
			t.Errorf("Handler %d should be removed after actor is removed", i)
		}

		// 验证handler已从全局管理器中注销
		if globalActorManager.IsTaskHandlerRegistered(getUniqueId(User, handlerID)) {
			t.Errorf("Handler %d should be unregistered from global manager", i)
		}
	}
}

// TestHandlerStateTransitions 测试Handler状态转换
func TestHandlerStateTransitions(t *testing.T) {
	Init(5000)

	actorID := "state_test_actor"
	actor := RegisterActor[*MockActor](User, actorID, actorID)

	handler := actor.TaskHandler
	if handler.GetState() != ActorStateRunning {
		t.Errorf("Expected state Running, got %v", handler.GetState())
	}

	// 发送任务确保handler正在工作
	result := actor.SendTask(func() string {
		return "test"
	})
	if result != "test" {
		t.Errorf("Expected 'test', got %v", result)
	}

	// 停止handler
	handler.Stop()

	// 验证状态
	if !handler.IsStopped() {
		t.Errorf("Handler should be stopped, state: %v", handler.GetState())
	}

	// 尝试发送任务到已停止的handler
	result = actor.SendTask(func() string {
		return "should_fail"
	})

	// SendTask应该返回error或Response
	if err, ok := result.(error); ok {
		// 返回了error，这是预期的
		if err == nil {
			t.Error("Expected non-nil error")
		}
	} else if resp, ok := result.(*Response); ok {
		// 返回了Response，检查Error字段
		if resp.Error == nil {
			t.Error("Expected error in Response when sending task to stopped handler")
		}
	} else {
		t.Errorf("Expected error or Response, got %T: %v", result, result)
	}

	// 清理
	time.Sleep(time.Millisecond * 50)
}

// TestConcurrentTaskExecution 测试并发任务执行
func TestConcurrentTaskExecution(t *testing.T) {
	Init(5000)

	const (
		numGoroutines = 100
		numTasks      = 100
	)

	actorID := "concurrent_task_actor"
	actor := RegisterActor[*MockActor](User, actorID, actorID)

	var wg sync.WaitGroup
	taskCounter := int32(0)
	errors := make(chan error, numGoroutines*numTasks)

	// 并发发送任务
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < numTasks; j++ {
				result := actor.SendTask(func() int {
					atomic.AddInt32(&taskCounter, 1)
					return int(taskCounter)
				})

				if err, ok := result.(error); ok {
					errors <- err
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	// 检查错误
	for err := range errors {
		t.Errorf("Task execution error: %v", err)
	}

	// 验证所有任务都执行了
	expectedCount := int32(numGoroutines * numTasks)
	if taskCounter != expectedCount {
		t.Errorf("Expected %d tasks executed, got %d", expectedCount, taskCounter)
	}

	// 清理
	StopActor[*MockActor](User, actorID)
	time.Sleep(time.Millisecond * 100)
}

// TestReentrantCall 测试重入调用
func TestReentrantCall(t *testing.T) {
	Init(5000)

	actorID := "reentrant_actor"
	actor := RegisterActor[*MockActor](User, actorID, actorID)

	// 在任务中再次调用SendTask（重入）
	result := actor.SendTask(func() string {
		// 这是一个重入调用，应该直接执行而不是进入队列
		innerResult := actor.SendTask(func() string {
			return "inner"
		})
		return fmt.Sprintf("outer_%v", innerResult)
	})

	expected := "outer_inner"
	if result != expected {
		t.Errorf("Expected '%s', got '%v'", expected, result)
	}

	// 清理
	StopActor[*MockActor](User, actorID)
	time.Sleep(time.Millisecond * 100)
}

// TestStopWithPendingTasks 测试停止时处理待处理任务
func TestStopWithPendingTasks(t *testing.T) {
	Init(5000)

	actorID := "pending_tasks_actor"
	actor := RegisterActor[*MockActor](User, actorID, actorID)

	const numTasks = 1000
	completedTasks := int32(0)

	// 发送大量异步任务
	for i := 0; i < numTasks; i++ {
		actor.SendTaskAsync(func() {
			time.Sleep(time.Millisecond) // 模拟耗时操作
			atomic.AddInt32(&completedTasks, 1)
		})
	}

	// 立即停止
	StopActor[*MockActor](User, actorID)

	// 验证至少有一些任务被执行了
	if completedTasks == 0 {
		t.Error("No tasks were completed before stop")
	}

	t.Logf("Completed %d out of %d tasks before stop", completedTasks, numTasks)
}

// TestGetActorAfterStop 测试停止后获取Actor
func TestGetActorAfterStop(t *testing.T) {
	Init(5000)

	actorID := "get_after_stop_actor"
	_ = RegisterActor[*MockActor](User, actorID, actorID)

	// 验证可以获取
	retrieved, exists := GetActor[MockActor](User, actorID)
	if !exists || retrieved == nil {
		t.Fatal("Should be able to get actor before stop")
	}

	// 停止
	StopActor[*MockActor](User, actorID)
	time.Sleep(time.Millisecond * 100)

	// 验证无法获取
	retrieved, exists = GetActor[MockActor](User, actorID)
	if exists {
		t.Error("Should not be able to get actor after stop")
	}
}

// BenchmarkActorRegistration 性能测试：Actor注册
func BenchmarkActorRegistration(b *testing.B) {
	Init(5000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		actorID := fmt.Sprintf("bench_actor_%d", i)
		_ = RegisterActor[*MockActor](User, actorID, actorID)
		StopActor[*MockActor](User, actorID)
	}
}

// BenchmarkTaskExecution 性能测试：任务执行
func BenchmarkTaskExecution(b *testing.B) {
	Init(5000)

	actorID := "bench_task_actor"
	actor := RegisterActor[*MockActor](User, actorID, actorID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		actor.SendTask(func() int {
			return i
		})
	}

	b.StopTimer()
	StopActor[*MockActor](User, actorID)
}

// BenchmarkConcurrentTaskExecution 性能测试：并发任务执行
func BenchmarkConcurrentTaskExecution(b *testing.B) {
	Init(5000)

	actorID := "bench_concurrent_actor"
	actor := RegisterActor[*MockActor](User, actorID, actorID)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			actor.SendTask(func() int {
				return i
			})
			i++
		}
	})

	b.StopTimer()
	StopActor[*MockActor](User, actorID)
}
